package output

import (
	"fmt"
	"io"

	"github.com/yeisme/pinax/internal/domain"
	noterender "github.com/yeisme/pinax/internal/render"
)

// RenderRenderedHTML exports the same sanitized projection without terminal markup.
func RenderRenderedHTML(w io.Writer, p domain.Projection) error {
	ApplyProjectionRedaction(&p)
	v, ok := p.Data.(noterender.View)
	if !ok {
		return fmt.Errorf("reference_unresolved: resolve the target before exporting HTML")
	}
	_, err := io.WriteString(w, noterender.HTML(v))
	return err
}
