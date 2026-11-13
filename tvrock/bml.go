package tvrock

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
)

// サーバー上でBMLを静的なHTMLに変換します。
// スクリプトとイベント属性を除去し、インタラクティブな処理は実行しません。
func convertBML(data []byte, encoding string) (string, error) {
	var input io.Reader = bytes.NewReader(data)
	if encoding == "" && !utf8.Valid(data) && !bytes.Contains(bytes.ToLower(data[:min(len(data), 256)]), []byte("encoding=")) {
		encoding = "euc-jp"
	}
	if encoding != "" {
		var err error
		input, err = charset.NewReaderLabel(encoding, input)
		if err != nil {
			return "", err
		}
	}
	dec := xml.NewDecoder(input)
	dec.CharsetReader = charset.NewReaderLabel
	if encoding != "" {
		// MIME文字セットは既に適用済みです。XML宣言によって結果として得られたUTF-8を
		// 二重にデコードしてはなりません（例：ヘッダーとXML宣言の双方がShift_JISである場合など）
		dec.CharsetReader = func(_ string, reader io.Reader) (io.Reader, error) { return reader, nil }
	}
	dec.Strict = false
	dec.Entity = map[string]string{"nbsp": "\u00a0", "copy": "©", "reg": "®"}
	var out strings.Builder
	out.WriteString(`<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data: blob:; media-src data: blob:; style-src 'unsafe-inline'; font-src data:; base-uri 'none'; form-action 'none'"><style>body{margin:0;background:black;color:white;font-family:monospace;width:960px;min-height:540px}a{color:inherit}img{object-fit:contain}</style></head><body>`)
	allowed := map[string]bool{"div": true, "p": true, "span": true, "a": true, "br": true, "img": true, "style": true, "pre": true, "ul": true, "ol": true, "li": true, "table": true, "tr": true, "td": true, "th": true, "input": true, "label": true, "button": true, "textarea": true, "select": true, "option": true, "h1": true, "h2": true, "h3": true}
	stack := []string{}
	skip := 0
	depth := 0
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("BML XML: %w", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			if depth > 256 {
				return "", fmt.Errorf("BML nesting limit")
			}
			name := strings.ToLower(t.Name.Local)
			if skip > 0 {
				skip++
				continue
			}
			if name == "script" || name == "title" || name == "bevent" || name == "beitem" {
				skip = 1
				continue
			}
			if name == "object" {
				name = "img"
			}
			if name == "body" {
				name = "div"
			}
			if !allowed[name] {
				stack = append(stack, "")
				continue
			}
			stack = append(stack, name)
			out.WriteByte('<')
			out.WriteString(name)
			for _, a := range t.Attr {
				k := strings.ToLower(a.Name.Local)
				if k == "src" || k == "data" {
					k = "data-bml-src"
				} else if k == "href" {
					k = "data-bml-href"
				} else if k != "id" && k != "class" && k != "style" && k != "width" && k != "height" && k != "alt" && k != "title" && k != "value" && k != "type" {
					continue
				}
				out.WriteByte(' ')
				out.WriteString(k)
				out.WriteString(`="`)
				out.WriteString(html.EscapeString(a.Value))
				out.WriteByte('"')
			}
			out.WriteString(">")
		case xml.EndElement:
			depth--
			if skip > 0 {
				skip--
				continue
			}
			if len(stack) == 0 {
				continue
			}
			name := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if name != "" && name != "img" && name != "br" && name != "input" {
				out.WriteString("</")
				out.WriteString(name)
				out.WriteByte('>')
			}
		case xml.CharData:
			if skip == 0 {
				if len(stack) > 0 && stack[len(stack)-1] == "style" {
					out.WriteString(strings.ReplaceAll(string(t), "<", "\\3c "))
				} else {
					out.WriteString(html.EscapeString(string(t)))
				}
			}
		}
		if out.Len() > 4*dsmModuleLimit {
			return "", fmt.Errorf("BML HTML size limit")
		}
	}
	out.WriteString("</body></html>")
	return out.String(), nil
}
