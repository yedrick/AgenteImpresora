package render

import (
	"strconv"
	"strings"
)

// attrs son los atributos de una etiqueta. Antes se partian por espacios, asi
// que un style="text-align: center" se rompia en pedazos y no se leia.
type attrs map[string]string

// parseAttrs lee pares nombre=valor respetando comillas simples y dobles.
func parseAttrs(s string) attrs {
	out := attrs{}
	i, n := 0, len(s)
	for i < n {
		for i < n && isSpace(s[i]) {
			i++
		}
		start := i
		for i < n && !isSpace(s[i]) && s[i] != '=' {
			i++
		}
		if i == start {
			i++
			continue
		}
		name := strings.ToLower(s[start:i])
		for i < n && isSpace(s[i]) {
			i++
		}
		if i >= n || s[i] != '=' {
			out[name] = "" // atributo sin valor, como <td nowrap>
			continue
		}
		i++ // saltar '='
		for i < n && isSpace(s[i]) {
			i++
		}
		if i >= n {
			out[name] = ""
			break
		}
		var value string
		if q := s[i]; q == '"' || q == '\'' {
			i++
			vs := i
			for i < n && s[i] != q {
				i++
			}
			value = s[vs:i]
			if i < n {
				i++
			}
		} else {
			vs := i
			for i < n && !isSpace(s[i]) {
				i++
			}
			value = s[vs:i]
		}
		out[name] = value
	}
	return out
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func (a attrs) get(name string) string { return a[name] }

func (a attrs) int(name string) int {
	v, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(a[name], "px")))
	if err != nil {
		return 0
	}
	return v
}

// style descompone el atributo style en sus propiedades.
func (a attrs) style() map[string]string {
	out := map[string]string{}
	for _, decl := range strings.Split(a["style"], ";") {
		k, v, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(k))] = strings.ToLower(strings.TrimSpace(v))
	}
	return out
}

// align devuelve la alineacion pedida por el atributo align o por el style.
func (a attrs) align() string {
	if v := strings.ToLower(strings.TrimSpace(a["align"])); v != "" {
		return v
	}
	return a.style()["text-align"]
}

// fontScale traduce font-size a un multiplicador ESC/POS. Las termicas solo
// escalan por numeros enteros, asi que los tamanos CSS se agrupan.
func fontScale(size string) (int, bool) {
	switch strings.TrimSpace(size) {
	case "":
		return 1, false
	case "xx-small", "x-small", "small", "smaller":
		return 1, true
	case "medium", "initial", "normal":
		return 1, true
	case "large", "larger":
		return 2, true
	case "x-large":
		return 3, true
	case "xx-large":
		return 4, true
	}
	// Valores en px o em: se agrupan por tramos.
	num := strings.TrimRight(strings.TrimSpace(size), "pxemrt%")
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 1, false
	}
	if strings.HasSuffix(size, "em") || strings.HasSuffix(size, "rem") {
		v *= 16
	}
	switch {
	case v >= 48:
		return 4, true
	case v >= 32:
		return 3, true
	case v >= 20:
		return 2, true
	default:
		return 1, true
	}
}
