package upload

import (
	"bytes"
	"fmt"
	"strings"
)

// Recheck applies the upload allow-list and magic-byte checks to saved bytes.
// Office containers also have their main part and content types checked again.
func Recheck(kind, name, mime string, data []byte) error {
	want, err := allowedType(name)
	if err != nil {
		return err
	}
	if want.kind != kind || want.mime != mime || int64(len(data)) > want.limit || len(data) < 100 {
		return fmt.Errorf("saved file metadata does not match the allowed type or size")
	}
	if !matchesMagic(want, data[:min(len(data), 64<<10)]) {
		return typeMismatch()
	}
	if strings.HasPrefix(mime, "application/vnd.openxmlformats") {
		return validateOffice(bytes.NewReader(data), int64(len(data)), mime)
	}
	return nil
}
