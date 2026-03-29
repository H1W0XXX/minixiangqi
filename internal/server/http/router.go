package httpserver

import "net/http"

// 这是一个薄封装：Server 里面包一层 Handler。
// 现在项目里其实可以只用 Handler，这个文件存在只是为了兼容老结构。
type Server struct {
	h http.Handler
}

// 你可以在别的地方用 NewServer()，但现在 main.go 已经直接用 NewHandler 了。
func NewServer() *Server {
	return &Server{h: NewHandler()}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.h.ServeHTTP(w, r)
}
