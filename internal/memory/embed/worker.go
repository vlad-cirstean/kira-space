package embed

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// MaxBatch is the most texts one worker request may carry.
const MaxBatch = 32

// encoder turns texts into L2-normalised vectors. The ONNX implementation is cgo-only.
type encoder interface {
	embed(texts []string) ([][]float32, error)
	close()
}

type workerHello struct {
	Ready bool   `json:"ready,omitempty"`
	Model string `json:"model,omitempty"`
	Dim   int    `json:"dim,omitempty"`
	Error string `json:"error,omitempty"`
}

type workerRequest struct {
	ID    int      `json:"id"`
	Texts []string `json:"texts"`
}

type workerReply struct {
	ID      int      `json:"id"`
	Vectors []string `json:"vectors,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// RunWorker is the `memory-embed` subcommand body: it loads the model, then serves NDJSON
// embedding requests on stdin/stdout until stdin closes. It returns the process exit code. stdout
// is the protocol; diagnostics go to stderr.
func RunWorker(args []string) int {
	fs := flag.NewFlagSet("memory-embed", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("model-dir", "", "directory holding the installed model files")
	socket := fs.String("socket", "", "serve many clients on this unix socket instead of stdin/stdout")
	idle := fs.Duration("idle", DefaultIdleTimeout, "with --socket: exit after this long without a request")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	out := bufio.NewWriter(os.Stdout)
	fail := func(err error) int {
		_ = json.NewEncoder(out).Encode(workerHello{Error: err.Error()})
		_ = out.Flush()
		return 2
	}
	spec := Default
	if *dir == "" || !Installed(*dir, spec) {
		return fail(errors.New("model is not installed"))
	}
	lib, err := RuntimeLib()
	if err != nil {
		return fail(err)
	}
	enc, err := newEncoder(lib, *dir, spec)
	if err != nil {
		return fail(err)
	}
	defer enc.close()
	if *socket != "" {
		return serveSocket(*socket, out, enc, spec, *idle)
	}
	return serve(os.Stdin, out, enc, spec)
}

// serve writes the hello line, then answers requests until in reaches EOF.
func serve(in io.Reader, out *bufio.Writer, enc encoder, spec Spec) int {
	return serveConn(in, out, spec, func(texts []string) ([][]float32, error) { return enc.embed(texts) })
}

// serveConn is the NDJSON protocol: hello, then one reply per request until in reaches EOF.
func serveConn(in io.Reader, out *bufio.Writer, spec Spec, embed func([]string) ([][]float32, error)) int {
	w := json.NewEncoder(out)
	if err := w.Encode(workerHello{Ready: true, Model: spec.ID, Dim: spec.Dim}); err != nil {
		return 1
	}
	if err := out.Flush(); err != nil {
		return 1
	}
	dec := json.NewDecoder(in)
	for {
		var req workerRequest
		if err := dec.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				return 0
			}
			fmt.Fprintln(os.Stderr, "memory-embed: bad request:", err)
			return 1
		}
		rep := workerReply{ID: req.ID}
		if n := len(req.Texts); n < 1 || n > MaxBatch {
			rep.Error = fmt.Sprintf("batch must hold 1 to %d texts", MaxBatch)
		} else if vecs, err := embed(req.Texts); err != nil {
			rep.Error = err.Error()
		} else {
			for _, v := range vecs {
				rep.Vectors = append(rep.Vectors, base64.StdEncoding.EncodeToString(Encode(v)))
			}
		}
		if err := w.Encode(rep); err != nil {
			return 1
		}
		if err := out.Flush(); err != nil {
			return 1
		}
	}
}
