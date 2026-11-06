// Package apifiles keeps the Api module's collections and environments as config files in
// <home>/api. The files are the source of truth; the api_* tables are a projection written only by
// this package. Secret values stay out of the files: a variable names a secret, whose sealed value
// lives in the app DB.
package apifiles

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// CollectionFile is <id>.collection.yaml.
type CollectionFile struct {
	Name      string     `json:"name"`
	Variables []Variable `json:"variables,omitempty"`
	Items     []Item     `json:"items"`
}

// EnvironmentFile is <id>.environment.yaml.
type EnvironmentFile struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Color       string     `json:"color,omitempty"`
	Variables   []Variable `json:"variables,omitempty"`
	// AgentAutoApproveReads skips the approval for body-less GET, HEAD and OPTIONS agent requests.
	AgentAutoApproveReads bool `json:"agentAutoApproveReads,omitempty"`
}

// Variable holds a plain Value, or a Secret reference (`${secret:name}`), never both.
type Variable struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	Secret      string `json:"secret,omitempty"`
	Description string `json:"description,omitempty"`
	// Agent makes the variable usable by agents; AgentHosts bounds where a secret may be sent.
	Agent      bool     `json:"agent,omitempty"`
	AgentHosts []string `json:"agentHosts,omitempty"`
}

// Item is a folder (Items), an HTTP request or a gRPC request.
type Item struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Items *[]Item `json:"items,omitempty"`
	HTTP  *HTTP   `json:"http,omitempty"`
	GRPC  *GRPC   `json:"grpc,omitempty"`
}

// Row is one header, urlencoded or metadata row. Enabled nil means on.
type Row struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
	Description string `json:"description,omitempty"`
}

// FormField is one form-data row.
type FormField struct {
	Name        string `json:"name"`
	Kind        string `json:"kind,omitempty"`
	Value       string `json:"value,omitempty"`
	Path        string `json:"path,omitempty"`
	FileName    string `json:"file_name,omitempty"`
	FileSize    int64  `json:"file_size,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
	Description string `json:"description,omitempty"`
}

// File is the binary body's one file.
type File struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
	Size int64  `json:"size,omitempty"`
}

// Body is a request body; every buffer is kept whatever the mode.
type Body struct {
	Mode       string      `json:"mode,omitempty"`
	Raw        string      `json:"raw,omitempty"`
	Code       string      `json:"code,omitempty"`
	Language   string      `json:"language,omitempty"`
	URLEncoded []Row       `json:"urlencoded,omitempty"`
	FormData   []FormField `json:"formdata,omitempty"`
	File       *File       `json:"file,omitempty"`
}

// HTTP is a saved HTTP request.
type HTTP struct {
	Method            string            `json:"method"`
	URL               string            `json:"url,omitempty"`
	Headers           []Row             `json:"headers,omitempty"`
	ParamDescriptions map[string]string `json:"param_descriptions,omitempty"`
	Body              *Body             `json:"body,omitempty"`
}

// GRPC is a saved gRPC request.
type GRPC struct {
	Target      string   `json:"target,omitempty"`
	TLS         string   `json:"tls,omitempty"`
	CAFile      string   `json:"ca_file,omitempty"`
	ServerName  string   `json:"server_name,omitempty"`
	Descriptor  string   `json:"descriptor,omitempty"`
	ProtoPath   string   `json:"proto_path,omitempty"`
	ImportPaths []string `json:"import_paths,omitempty"`
	Service     string   `json:"service,omitempty"`
	Method      string   `json:"method,omitempty"`
	Message     string   `json:"message,omitempty"`
	Metadata    []Row    `json:"metadata,omitempty"`
}

const (
	bodyModeNone  = "none"
	langJSON      = "json"
	tlsOn         = "tls"
	descReflect   = "reflection"
	formKindText  = "text"
	defaultMethod = "GET"
)

func enabledPtr(on bool) *bool {
	if on {
		return nil
	}
	off := false
	return &off
}

func isEnabled(p *bool) bool { return p == nil || *p }

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// Saved converts the file form to the stored request. Slices and the map are never nil, so the
// stored JSON holds [] and {} like every row saved before.
func (h HTTP) Saved() model.SavedRequest {
	r := model.SavedRequest{
		Method: h.Method, URL: h.URL, BodyMode: bodyModeNone, CodeLanguage: langJSON,
		Headers: []model.SavedHeader{}, URLEncoded: []model.SavedField{}, FormData: []model.SavedFormField{},
		ParamDescriptions: map[string]string{},
	}
	for _, row := range h.Headers {
		r.Headers = append(r.Headers, model.SavedHeader{Name: row.Name, Value: row.Value, Enabled: isEnabled(row.Enabled), Description: row.Description})
	}
	for k, v := range h.ParamDescriptions {
		r.ParamDescriptions[k] = v
	}
	if b := h.Body; b != nil {
		r.BodyMode = orDefault(b.Mode, bodyModeNone)
		r.Body, r.Code, r.CodeLanguage = b.Raw, b.Code, orDefault(b.Language, langJSON)
		for _, row := range b.URLEncoded {
			r.URLEncoded = append(r.URLEncoded, model.SavedField{Name: row.Name, Value: row.Value, Enabled: isEnabled(row.Enabled), Description: row.Description})
		}
		for _, f := range b.FormData {
			r.FormData = append(r.FormData, model.SavedFormField{
				Name: f.Name, Kind: orDefault(f.Kind, formKindText), Value: f.Value, Path: f.Path, FileName: f.FileName,
				FileSize: f.FileSize, ContentType: f.ContentType, Enabled: isEnabled(f.Enabled), Description: f.Description,
			})
		}
		if b.File != nil {
			r.BinaryFile = &model.SavedFile{Path: b.File.Path, Name: b.File.Name, Size: b.File.Size}
		}
	}
	return r
}

// HTTPOf converts a stored request to the file form, dropping defaults.
func HTTPOf(r model.SavedRequest) HTTP {
	h := HTTP{Method: orDefault(r.Method, defaultMethod), URL: r.URL}
	for _, row := range r.Headers {
		h.Headers = append(h.Headers, Row{Name: row.Name, Value: row.Value, Enabled: enabledPtr(row.Enabled), Description: row.Description})
	}
	if len(r.ParamDescriptions) > 0 {
		h.ParamDescriptions = map[string]string{}
		for k, v := range r.ParamDescriptions {
			h.ParamDescriptions[k] = v
		}
	}
	b := Body{Raw: r.Body, Code: r.Code}
	if r.BodyMode != bodyModeNone {
		b.Mode = r.BodyMode
	}
	if r.CodeLanguage != langJSON {
		b.Language = r.CodeLanguage
	}
	for _, row := range r.URLEncoded {
		b.URLEncoded = append(b.URLEncoded, Row{Name: row.Name, Value: row.Value, Enabled: enabledPtr(row.Enabled), Description: row.Description})
	}
	for _, f := range r.FormData {
		kind := f.Kind
		if kind == formKindText {
			kind = ""
		}
		b.FormData = append(b.FormData, FormField{
			Name: f.Name, Kind: kind, Value: f.Value, Path: f.Path, FileName: f.FileName, FileSize: f.FileSize,
			ContentType: f.ContentType, Enabled: enabledPtr(f.Enabled), Description: f.Description,
		})
	}
	if r.BinaryFile != nil {
		b.File = &File{Path: r.BinaryFile.Path, Name: r.BinaryFile.Name, Size: r.BinaryFile.Size}
	}
	if b.Mode != "" || b.Raw != "" || b.Code != "" || b.Language != "" || len(b.URLEncoded) > 0 || len(b.FormData) > 0 || b.File != nil {
		h.Body = &b
	}
	return h
}

// Saved converts the file form to the stored gRPC request.
func (g GRPC) Saved() model.SavedGrpcRequest {
	r := model.SavedGrpcRequest{
		Target: g.Target, TLSMode: orDefault(g.TLS, tlsOn), CAFile: g.CAFile, ServerName: g.ServerName,
		DescriptorMode: orDefault(g.Descriptor, descReflect), ProtoPath: g.ProtoPath, ImportPaths: []string{},
		Service: g.Service, Method: g.Method, Message: g.Message, Metadata: []model.SavedGrpcMetaRow{},
	}
	r.ImportPaths = append(r.ImportPaths, g.ImportPaths...)
	for _, row := range g.Metadata {
		r.Metadata = append(r.Metadata, model.SavedGrpcMetaRow{Name: row.Name, Value: row.Value, Enabled: isEnabled(row.Enabled), Description: row.Description})
	}
	return r
}

// GRPCOf converts a stored gRPC request to the file form, dropping defaults.
func GRPCOf(r model.SavedGrpcRequest) GRPC {
	g := GRPC{
		Target: r.Target, CAFile: r.CAFile, ServerName: r.ServerName, ProtoPath: r.ProtoPath, ImportPaths: r.ImportPaths,
		Service: r.Service, Method: r.Method, Message: r.Message,
	}
	if r.TLSMode != tlsOn {
		g.TLS = r.TLSMode
	}
	if r.DescriptorMode != descReflect {
		g.Descriptor = r.DescriptorMode
	}
	for _, row := range r.Metadata {
		g.Metadata = append(g.Metadata, Row{Name: row.Name, Value: row.Value, Enabled: enabledPtr(row.Enabled), Description: row.Description})
	}
	return g
}
