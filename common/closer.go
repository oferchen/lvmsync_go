package common

import (
	"fmt"
	"io"
)

// CloseWithErr closes the provided io.Closer and augments errp if closing fails.
// If errp points to a nil error, the close error is wrapped with msg.
// If errp already holds an error, the close error is appended to its message.
func CloseWithErr(closer io.Closer, errp *error, msg string) {
	if closer == nil {
		return
	}
	if closeErr := closer.Close(); closeErr != nil {
		if errp == nil {
			return
		}
		if *errp == nil {
			*errp = fmt.Errorf("%s: %w", msg, closeErr)
		} else {
			*errp = fmt.Errorf("%v; %s: %w", *errp, msg, closeErr)
		}
	}
}
