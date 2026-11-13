//go:build windows

package main

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"
	"image/png"
	"unsafe"
)

//go:embed app/public/image/tvrockgo.png
var trayPNG []byte

//go:embed app/src/assets/tvrockgo_sleep.png
var traySleepPNG []byte

// ソースとなるPNGから32ビットのWindowsアイコンリソースを構築する。PNGを埋め込むことで
// トレイ機能が静的ファイルビルドや外部のアイコンファイルに依存しないようにする。
func createTrayIcon() (uintptr, error) {
	return createTrayIconFromPNG(trayPNG)
}

func createTrayIconFromPNG(data []byte) (uintptr, error) {
	source, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	const size = 32
	const maskStride = 4
	dib := make([]byte, 40+size*size*4+maskStride*size)
	binary.LittleEndian.PutUint32(dib[0:], 40)
	binary.LittleEndian.PutUint32(dib[4:], size)
	binary.LittleEndian.PutUint32(dib[8:], size*2)
	binary.LittleEndian.PutUint16(dib[12:], 1)
	binary.LittleEndian.PutUint16(dib[14:], 32)
	bounds := source.Bounds()
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			r, g, b, a := source.At(bounds.Min.X+x*bounds.Dx()/size, bounds.Min.Y+y*bounds.Dy()/size).RGBA()
			offset := 40 + ((size-1-y)*size+x)*4
			// RGBAはネイティブのアルファ付きアイコンと整合するようプリマルチプライ（事前乗算）された成分を返します
			dib[offset], dib[offset+1], dib[offset+2], dib[offset+3] = byte(b>>8), byte(g>>8), byte(r>>8), byte(a>>8)
			if a == 0 {
				dib[40+size*size*4+(size-1-y)*maskStride+x/8] |= 0x80 >> (x % 8)
			}
		}
	}
	icon, _, err := user32.NewProc("CreateIconFromResourceEx").Call(uintptr(unsafe.Pointer(&dib[0])), uintptr(len(dib)), 1, 0x00030000, size, size, 0)
	if icon == 0 {
		return 0, fmt.Errorf("トレイアイコン作成: %w", err)
	}
	return icon, nil
}
