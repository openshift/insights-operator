package types

import "fmt"

// Warning represents warnings which happened during gathering/writing data.
// Warnings are also written to logs and stored in the metadata but in the different field
type Warning struct {
	UnderlyingValue error
}

func (w *Warning) Error() string {
	return fmt.Sprintf("warning: %v", w.UnderlyingValue.Error())
}
