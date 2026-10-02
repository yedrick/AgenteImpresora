package render

import (
	"bytes"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"regexp"
	"strings"
)

type Renderer struct{}

func NewRenderer() *Renderer { return &Renderer{} }

func (r *Renderer) HTMLToPNG(markup string, width int) ([]byte, error) {
	img, err := r.HTMLToImage(markup, width)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (r *Renderer) HTMLToImage(markup string, width int) (image.Image, error) {
	if strings.TrimSpace(markup) == "" {
		return nil, fmt.Errorf("html is required")
	}
	if width <= 0 {
		width = 576
	}
	text := html.UnescapeString(stripTags(markup))
	lines := wrapText(text, max(24, width/9))
	height := max(120, 28+len(lines)*18)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	drawTextBlocks(img, lines)
	return img, nil
}

func stripTags(s string) string {
	reBreak := regexp.MustCompile(`(?i)<\s*(br|p|div|tr|li|h[1-6])[^>]*>`)
	s = reBreak.ReplaceAllString(s, "\n")
	re := regexp.MustCompile(`<[^>]+>`)
	s = re.ReplaceAllString(s, " ")
	reSpace := regexp.MustCompile(`[ \t]+`)
	s = reSpace.ReplaceAllString(s, " ")
	reNL := regexp.MustCompile(`\n\s+`)
	s = reNL.ReplaceAllString(s, "\n")
	return strings.TrimSpace(s)
}

func wrapText(s string, cols int) []string {
	var lines []string
	for _, raw := range strings.Split(s, "\n") {
		words := strings.Fields(raw)
		line := ""
		for _, word := range words {
			if len(line)+len(word)+1 > cols && line != "" {
				lines = append(lines, line)
				line = word
			} else if line == "" {
				line = word
			} else {
				line += " " + word
			}
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func drawTextBlocks(img *image.RGBA, lines []string) {
	black := color.RGBA{A: 255}
	for i, line := range lines {
		y := 14 + i*18
		for j, r := range line {
			x := 10 + j*8
			if x+6 >= img.Bounds().Dx() {
				break
			}
			drawRune(img, x, y, r, black)
		}
	}
}

func drawRune(img *image.RGBA, x, y int, r rune, c color.Color) {
	if r == ' ' {
		return
	}
	for row := 0; row < 9; row++ {
		for col := 0; col < 5; col++ {
			if row == 0 || row == 8 || col == 0 || col == 4 || ((int(r)+row+col)%7 == 0) {
				img.Set(x+col, y+row, c)
			}
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
