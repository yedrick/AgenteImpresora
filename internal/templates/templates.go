package templates

import (
	"bytes"
	"html/template"
	"path/filepath"
)

func Render(dir, name string, data map[string]any) (string, error) {
	tpl, err := template.ParseFiles(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
