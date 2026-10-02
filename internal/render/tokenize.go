package render

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"
)

// rawTextTags son elementos cuyo contenido NO es texto para imprimir. Sin
// esto, el CSS de cualquier HTML real salia impreso en el ticket.
var rawTextTags = map[string]bool{"style": true, "script": true, "title": true, "head": true}

// voidTags no tienen cierre.
var voidTags = map[string]bool{
	"br": true, "hr": true, "img": true, "input": true, "meta": true,
	"link": true, "col": true, "feed": true,
}

func tokenizeHTML(markup string) []htmlToken {
	var tokens []htmlToken
	i, n := 0, len(markup)

	for i < n {
		if markup[i] != '<' {
			end := strings.IndexByte(markup[i:], '<')
			if end == -1 {
				tokens = append(tokens, htmlToken{text: markup[i:]})
				break
			}
			if end > 0 {
				tokens = append(tokens, htmlToken{text: markup[i : i+end]})
			}
			i += end
			continue
		}

		if i+1 < n && markup[i+1] == '!' {
			// Comentario: se consume hasta "-->".
			if strings.HasPrefix(markup[i:], "<!--") {
				end := strings.Index(markup[i+4:], "-->")
				if end == -1 {
					break
				}
				i += 4 + end + 3
				continue
			}
			// Declaracion tipo <!DOCTYPE html>: solo hasta el '>'. Tratarla
			// como comentario descartaba el documento entero.
			end := strings.IndexByte(markup[i:], '>')
			if end == -1 {
				break
			}
			i += end + 1
			continue
		}

		end := strings.IndexByte(markup[i:], '>')
		if end == -1 {
			tokens = append(tokens, htmlToken{text: markup[i:]})
			break
		}
		content := markup[i+1 : i+end]
		i += end + 1

		switch {
		case strings.HasPrefix(content, "/"):
			tag, _ := parseTag(content[1:])
			tokens = append(tokens, htmlToken{tag: tag, isOpen: false})
		case strings.HasSuffix(content, "/"):
			tag, a := parseTag(content[:len(content)-1])
			tokens = append(tokens, htmlToken{tag: tag, attrs: a, isSelf: true})
		default:
			tag, a := parseTag(content)
			self := voidTags[tag]
			tokens = append(tokens, htmlToken{tag: tag, attrs: a, isOpen: !self, isSelf: self})
			if rawTextTags[tag] {
				if skip := indexCloseTag(markup[i:], tag); skip >= 0 {
					i += skip
				} else {
					i = n
				}
			}
		}
	}
	return tokens
}

// indexCloseTag devuelve el desplazamiento del "</tag" mas cercano, sin
// distinguir mayusculas.
func indexCloseTag(s, tag string) int {
	return strings.Index(strings.ToLower(s), "</"+tag)
}

func parseTag(s string) (string, attrs) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", attrs{}
	}
	idx := strings.IndexFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if idx < 0 {
		return strings.ToLower(s), attrs{}
	}
	return strings.ToLower(s[:idx]), parseAttrs(s[idx+1:])
}

// decodeImageData lee una imagen desde un data URI o desde base64 a secas.
func decodeImageData(src string) (image.Image, error) {
	src = strings.TrimSpace(src)
	if i := strings.Index(src, ","); strings.HasPrefix(src, "data:") && i >= 0 {
		src = src[i+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(src)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	return img, err
}
