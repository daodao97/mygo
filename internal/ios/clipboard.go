//go:build ios && cgo

package ios

import (
	"slices"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/transfer"
)

// UIKit copies plain text immediately; no lazy provider remains owned by Go.
func (c clipboard) WriteData(data transfer.Data, released func()) error {
	if len(data.Items()) == 0 {
		c.Clear()
	} else {
		for _, f := range data.Formats() {
			if f != transfer.Text {
				return platform.ErrUnsupported
			}
		}
		text, err := data.Read(transfer.Text)
		if err != nil {
			return err
		}
		c.WriteText(string(text))
	}
	if released != nil {
		released()
	}
	return nil
}

func (c clipboard) ReadData(formats []transfer.Format) (transfer.Data, error) {
	if formats != nil && !slices.Contains(formats, transfer.Text) {
		return transfer.Data{}, transfer.ErrFormat
	}
	text := c.ReadText()
	if text == "" {
		if formats != nil {
			return transfer.Data{}, transfer.ErrFormat
		}
		return transfer.Data{}, nil
	}
	return transfer.TextData(text), nil
}
func (c clipboard) Formats() []transfer.Format {
	if c.ReadText() != "" {
		return []transfer.Format{transfer.Text}
	}
	return nil
}
func (clipboard) Flush() error { return nil }
func (clipboard) Close()       {}
