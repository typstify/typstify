package lsp

// rpcMethodCompileStatus custom notification. Tinymist sends it when compileStatus initialization option is set to enable.
const rpcMethodCompileStatus = "tinymist/compileStatus"

type CompileState string

const (
	CompileStateCompiling CompileState = "compiling"
	CompileStateSuccess   CompileState = "compileSuccess"
	CompileStateError     CompileState = "compileError"
)

// CompileStatus is the compilation status of the primary document reported by tinymist.
type CompileStatus struct {
	Status CompileState `json:"status"`
	Path string `json:"path"` // path
	PageCount int `json:"pageCount"` // Number of pages
}

// CompileStatus returns if compilation finished, or nil, if tinymis not reported one.
func (c *Client) CompileStatus() *CompileStatus {
	return c.compileStatus.Load()
}

// OnCompileStatusChanged sets callback which is called when tinymist reports new compile status - from the LSP connection goroutine.
func (c *Client) OnCompileStatusChanged(fn func()) {
	c.onCompileStatus.Store(&fn)
}
