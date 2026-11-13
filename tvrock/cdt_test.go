package tvrock

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCDTPNGAndSave(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	im.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "cdt")
	p := PacketCDT{Sid: 101, LogoType: 5, Data: hex.EncodeToString(b.Bytes())}
	for i := 0; i < 2; i++ {
		if err := saveCDT(dir, p); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "101_5"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err == nil {
		t.Fatal("stored logo is still readable as PNG")
	}
	xorCDT(data)
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds() != im.Bounds() {
		t.Fatal("dimensions changed")
	}
	if color.NRGBAModel.Convert(decoded.At(0, 0)) != im.At(0, 0) {
		t.Fatal("pixel changed")
	}
	p.LogoType = 6
	if saveCDT(dir, p) == nil {
		t.Fatal("bad metadata accepted")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatal("temporary files leaked")
	}
}
func TestCDTARIBPalette(t *testing.T) {
	im := image.NewPaletted(image.Rect(0, 0, 2, 1), color.Palette{color.Black, color.White, color.Black, color.Black, color.Black, color.Black, color.Black, color.Black, color.Transparent})
	im.Pix[0] = 1
	im.Pix[1] = 8
	var b bytes.Buffer
	png.Encode(&b, im)
	raw := append([]byte{}, b.Bytes()[:8]...)
	for pos := 8; pos < b.Len(); {
		n := int(binary.BigEndian.Uint32(b.Bytes()[pos:]))
		kind := string(b.Bytes()[pos+4 : pos+8])
		if kind != "PLTE" && kind != "tRNS" {
			raw = append(raw, b.Bytes()[pos:pos+n+12]...)
		}
		pos += n + 12
	}
	decoded, err := decodeLogo(raw)
	if err != nil {
		t.Fatal(err)
	}
	r, g, blue, a := decoded.At(0, 0).RGBA()
	if r != 65535 || g != 0 || blue != 0 || a != 65535 {
		t.Fatal("default red incorrect")
	}
	_, _, _, a = decoded.At(1, 0).RGBA()
	if a != 0 {
		t.Fatal("transparent index incorrect")
	}
	raw[len(raw)-1] ^= 1
	if _, err = decodeLogo(raw); err == nil {
		t.Fatal("bad CRC accepted")
	}
}
func TestCDTBMPAndMalformed(t *testing.T) {
	b := make([]byte, 58)
	copy(b, "BM")
	binary.LittleEndian.PutUint32(b[10:], 54)
	binary.LittleEndian.PutUint32(b[14:], 40)
	binary.LittleEndian.PutUint32(b[18:], 1)
	binary.LittleEndian.PutUint32(b[22:], 1)
	binary.LittleEndian.PutUint16(b[26:], 1)
	binary.LittleEndian.PutUint16(b[28:], 24)
	b[56] = 255
	im, err := decodeLogo(b)
	if err != nil {
		t.Fatal(err)
	}
	r, g, blue, _ := im.At(0, 0).RGBA()
	if r != 65535 || g != 0 || blue != 0 {
		t.Fatal("BGR conversion")
	}
	for n := 0; n < len(b); n++ {
		if _, err := decodeLogo(b[:n]); err == nil {
			t.Fatalf("truncated BMP accepted %d", n)
		}
	}
	binary.LittleEndian.PutUint32(b[18:], 100000)
	if _, err := decodeLogo(b); err == nil {
		t.Fatal("oversize accepted")
	}
}

func TestCDTFileHandler(t *testing.T) {
	staticDir := t.TempDir()
	dir := filepath.Join(staticDir, "cdt")
	im := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	im.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	if err := saveCDT(dir, PacketCDT{Sid: 101, LogoType: 1, Data: hex.EncodeToString(b.Bytes())}); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"102_1.png": b.Bytes(),       // 既存PNGをそのまま利用。
		"101_1.png": []byte("stale"), // 新形式を優先。
		"103_1":     []byte("broken"),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(staticDir, "app.js"), []byte("app"), 0600); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(cdtFileHandler(dir))
	r.StaticFS("/static", http.Dir(staticDir))
	for _, name := range []string{"101_1", "101_1.png", "102_1", "102_1.png"} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/static/cdt/"+name, nil))
			if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" {
				t.Fatalf("response: %d %v", w.Code, w.Header())
			}
			decoded, err := png.Decode(w.Body)
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Bounds() != im.Bounds() || color.NRGBAModel.Convert(decoded.At(0, 0)) != im.At(0, 0) {
				t.Fatal("served image changed")
			}
		})
	}
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{http.MethodHead, "/static/cdt/101_1", http.StatusOK},
		{http.MethodGet, "/static/cdt/999_1", http.StatusNotFound},
		{http.MethodGet, "/static/cdt/101_6", http.StatusNotFound},
		{http.MethodGet, "/static/cdt/0_1", http.StatusNotFound},
		{http.MethodGet, "/static/cdt/../app.js", http.StatusNotFound},
		{http.MethodGet, "/static/cdt/", http.StatusNotFound},
		{http.MethodGet, "/static/cdt/103_1", http.StatusInternalServerError},
		{http.MethodPost, "/static/cdt/101_1", http.StatusMethodNotAllowed},
		{http.MethodGet, "/static/app.js", http.StatusOK},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status {
			t.Errorf("%s %s: got %d, want %d", tc.method, tc.path, w.Code, tc.status)
		}
		if tc.method == http.MethodHead && (w.Body.Len() != 0 || w.Header().Get("Content-Type") != "image/png") {
			t.Error("HEAD should return PNG headers without a body")
		}
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/cdt/101_1", nil)
	req.Header.Set("Range", "bytes=0-7")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusPartialContent || w.Body.String() != "\x89PNG\r\n\x1a\n" {
		t.Fatal("range response was not restored PNG")
	}
}
