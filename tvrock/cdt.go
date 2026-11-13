package tvrock

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// データは16進数でエンコードされているため既存のテキストベースのWebSocketプロトコルは変更されません。
type PacketCDT struct {
	Sid         uint32
	Onid        uint16
	LogoType    uint8
	LogoId      uint16
	LogoVersion uint16
	Data        string
}

// webserver.go と同じ静的ファイルのルートを使用する（別の作業ディレクトリから起動された場合も含む）
func receiveCDT(p PacketCDT) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return saveCDT(filepath.Join(filepath.Dir(exe), "static", "cdt"), p)
}

func saveCDT(dir string, p PacketCDT) error {
	if p.Sid == 0 || p.LogoType > 5 || p.LogoId > 511 || p.LogoVersion > 4095 || len(p.Data) > 131070 {
		return fmt.Errorf("invalid CDT metadata")
	}
	data, err := hex.DecodeString(p.Data)
	if err != nil {
		return err
	}
	img, err := decodeLogo(data)
	if err != nil {
		return err
	}
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, img); err != nil {
		return err
	}
	xorCDT(encoded.Bytes())
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".cdt-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(encoded.Bytes())
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(dir, fmt.Sprintf("%d_%d", p.Sid, p.LogoType)))
}

// xorCDTは保存用の簡易難読化。同じ処理でPNGに復元できる（暗号化ではない）
func xorCDT(data []byte) {
	for i := range data {
		data[i] ^= 0xa5
	}
}

func decodeLogo(data []byte) (image.Image, error) {
	if bytes.HasPrefix(data, []byte("BM")) {
		return decodeLogoBMP(data)
	}
	data, err := logoPNGPalette(data)
	if err != nil {
		return nil, err
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 256 || cfg.Height > 256 {
		return nil, fmt.Errorf("invalid logo dimensions")
	}
	return png.Decode(bytes.NewReader(data))
}

func pngChunk(kind string, data []byte) []byte {
	b := make([]byte, len(data)+12)
	binary.BigEndian.PutUint32(b, uint32(len(data)))
	copy(b[4:], kind)
	copy(b[8:], data)
	binary.BigEndian.PutUint32(b[len(b)-4:], crc32.ChecksumIEEE(b[4:len(b)-4]))
	return b
}

// ARIB STD-B21 PNGでは、PLTE/tRNSを省略し、STD-B24のデフォルトCLUTを使用することができます。
func logoPNGPalette(b []byte) ([]byte, error) {
	if len(b) < 33 || string(b[:8]) != "\x89PNG\r\n\x1a\n" {
		return nil, fmt.Errorf("unsupported logo format")
	}
	palette, transparency, idat := false, false, -1
	for pos := 8; pos < len(b); {
		if len(b)-pos < 12 {
			return nil, fmt.Errorf("truncated PNG chunk")
		}
		n := int(binary.BigEndian.Uint32(b[pos:]))
		if n > len(b)-pos-12 {
			return nil, fmt.Errorf("invalid PNG chunk length")
		}
		chunk := b[pos : pos+n+12]
		if crc32.ChecksumIEEE(chunk[4:n+8]) != binary.BigEndian.Uint32(chunk[n+8:]) {
			return nil, fmt.Errorf("PNG CRC mismatch")
		}
		switch string(chunk[4:8]) {
		case "PLTE":
			palette = true
		case "tRNS":
			transparency = true
		case "IDAT":
			if idat < 0 {
				idat = pos
			}
		}
		pos += n + 12
	}
	if palette || b[25] != 3 {
		return b, nil
	}
	if idat < 0 || string(b[12:16]) != "IHDR" || binary.BigEndian.Uint32(b[8:12]) != 13 {
		return nil, fmt.Errorf("missing PNG header/data")
	}
	colors := []color.NRGBA{{0, 0, 0, 255}, {255, 0, 0, 255}, {0, 255, 0, 255}, {255, 255, 0, 255}, {0, 0, 255, 255}, {255, 0, 255, 255}, {0, 255, 255, 255}, {255, 255, 255, 255}, {0, 0, 0, 0}, {170, 0, 0, 255}, {0, 170, 0, 255}, {170, 170, 0, 255}, {0, 0, 170, 255}, {170, 0, 170, 255}, {0, 170, 170, 255}, {170, 170, 170, 255}}
	for _, r := range []uint8{0, 85, 170, 255} {
		for _, g := range []uint8{0, 85, 170, 255} {
			for _, blue := range []uint8{0, 85, 170, 255} {
				found := false
				for _, c := range colors {
					if c.R == r && c.G == g && c.B == blue {
						found = true
						break
					}
				}
				if !found {
					colors = append(colors, color.NRGBA{r, g, blue, 255})
				}
			}
		}
	}
	for i := 65; i < 128; i++ {
		index := i - 64
		if i <= 72 {
			index = i - 65
		}
		c := colors[index]
		c.A = 128
		colors = append(colors, c)
	}
	if b[24] != 1 && b[24] != 2 && b[24] != 4 && b[24] != 8 {
		return nil, fmt.Errorf("invalid PNG bit depth")
	}
	count := 1 << b[24]
	if count > 128 {
		count = 128
	}
	var plte, alpha []byte
	for _, c := range colors[:count] {
		plte = append(plte, c.R, c.G, c.B)
		alpha = append(alpha, c.A)
	}
	// PLTEが既存のtRNSより前に来るように、IHDRの直後に挿入する。
	out := append([]byte{}, b[:33]...)
	out = append(out, pngChunk("PLTE", plte)...)
	if !transparency {
		out = append(out, pngChunk("tRNS", alpha)...)
	}
	return append(out, b[33:]...), nil
}

// Windows BMP、非圧縮インデックスカラー（1/4/8 bpp）またはBGR（24/32 bpp）
func decodeLogoBMP(b []byte) (image.Image, error) {
	bad := func() (image.Image, error) { return nil, fmt.Errorf("unsupported or malformed BMP logo") }
	if len(b) < 54 {
		return bad()
	}
	u32 := func(p int) uint32 { return binary.LittleEndian.Uint32(b[p:]) }
	header, offset := int(u32(14)), int(u32(10))
	w, h := int(int32(u32(18))), int(int32(u32(22)))
	top := h < 0
	if top {
		h = -h
	}
	bits := int(binary.LittleEndian.Uint16(b[28:]))
	if header < 40 || header > len(b)-14 || offset < 14+header || offset > len(b) || w < 1 || h < 1 || w > 256 || h > 256 || binary.LittleEndian.Uint16(b[26:]) != 1 || u32(30) != 0 {
		return bad()
	}
	if bits != 1 && bits != 4 && bits != 8 && bits != 24 && bits != 32 {
		return bad()
	}
	stride := ((w*bits + 31) / 32) * 4
	if stride*h > len(b)-offset {
		return bad()
	}
	var palette []color.NRGBA
	if bits <= 8 {
		count := int(u32(46))
		if count == 0 {
			count = 1 << bits
		}
		if count > 1<<bits || count*4 > offset-14-header {
			return bad()
		}
		for i := 0; i < count; i++ {
			p := 14 + header + i*4
			palette = append(palette, color.NRGBA{b[p+2], b[p+1], b[p], 255})
		}
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy := h - 1 - y
		if top {
			sy = y
		}
		row := b[offset+sy*stride:]
		for x := 0; x < w; x++ {
			var c color.NRGBA
			if bits <= 8 {
				index := int(row[x*bits/8]>>uint(8-bits-(x*bits%8))) & ((1 << bits) - 1)
				if index >= len(palette) {
					return bad()
				}
				c = palette[index]
			} else {
				p := x * (bits / 8)
				c = color.NRGBA{row[p+2], row[p+1], row[p], 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img, nil
}

// staticのワイルドカードルートより前に局ロゴを復元する。
// 既存PNGは再受信まで読み取り専用で利用し、旧URLにも対応する。
func cdtFileHandler(dir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		const prefix = "/static/cdt/"
		if !strings.HasPrefix(c.Request.URL.Path, prefix) {
			c.Next()
			return
		}
		c.Abort()
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Header("Allow", "GET, HEAD")
			c.Status(http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimSuffix(strings.TrimPrefix(c.Request.URL.Path, prefix), ".png")
		parts := strings.Split(name, "_")
		if len(parts) != 2 {
			c.Status(http.StatusNotFound)
			return
		}
		sid, sidErr := strconv.ParseUint(parts[0], 10, 32)
		logoType, typeErr := strconv.ParseUint(parts[1], 10, 8)
		if sidErr != nil || typeErr != nil || sid == 0 || logoType > 5 {
			c.Status(http.StatusNotFound)
			return
		}
		// 数値として検証した名前だけを使用し、ディレクトリ外の参照を防ぐ。
		name = strconv.FormatUint(sid, 10) + "_" + strconv.FormatUint(logoType, 10)
		f, err := os.Open(filepath.Join(dir, name))
		encoded := true
		if os.IsNotExist(err) {
			f, err = os.Open(filepath.Join(dir, name+".png"))
			encoded = false
		}
		if err != nil {
			if os.IsNotExist(err) {
				c.Status(http.StatusNotFound)
			} else {
				c.Status(http.StatusInternalServerError)
			}
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() {
			c.Status(http.StatusInternalServerError)
			return
		}
		data, err := io.ReadAll(f)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		if encoded {
			xorCDT(data)
		}
		if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Header("Content-Type", "image/png")
		http.ServeContent(c.Writer, c.Request, name+".png", info.ModTime(), bytes.NewReader(data))
	}
}
