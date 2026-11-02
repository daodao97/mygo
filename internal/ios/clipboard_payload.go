//go:build (ios && cgo) || mygo_ios_clipboard

package ios

import (
	"errors"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/transfer"
)

// Materialize every representation before touching UIKit. Failure leaves the
// existing pasteboard unchanged, including when a rich-text provider fails.
type clipboardPayload struct {
	text, html, markdown *string
	png                  []byte
}

func prepareClipboard(data transfer.Data) (clipboardPayload, error) {
	var out clipboardPayload
	for _, format := range data.Formats() {
		switch format {
		case transfer.Text, transfer.HTML, transfer.Markdown, transfer.PNG:
		default:
			return clipboardPayload{}, platform.ErrUnsupported
		}
		value, err := data.Read(format)
		if err != nil {
			return clipboardPayload{}, err
		}
		switch format {
		case transfer.Text:
			text := string(value)
			out.text = &text
		case transfer.HTML:
			html := string(value)
			out.html = &html
		case transfer.Markdown:
			markdown := string(value)
			out.markdown = &markdown
		case transfer.PNG:
			if len(value) == 0 {
				return clipboardPayload{}, errors.New("mygo: empty clipboard image")
			}
			out.png = value
		}
	}
	return out, nil
}
