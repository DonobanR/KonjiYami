package rpc

import (
	"context"

	"konjiyami/protobuf/commonpb"
	"konjiyami/protobuf/konjiyamipb"
)

// RegisterExtension registers a new extension in the implant
func (rpc *Server) RegisterExtension(ctx context.Context, req *konjiyamipb.RegisterExtensionReq) (*konjiyamipb.RegisterExtension, error) {
	resp := &konjiyamipb.RegisterExtension{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// ListExtensions lists the registered extensions
func (rpc *Server) ListExtensions(ctx context.Context, req *konjiyamipb.ListExtensionsReq) (*konjiyamipb.ListExtensions, error) {
	resp := &konjiyamipb.ListExtensions{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// CallExtension calls a specific export of the loaded extension
func (rpc *Server) CallExtension(ctx context.Context, req *konjiyamipb.CallExtensionReq) (*konjiyamipb.CallExtension, error) {
	resp := &konjiyamipb.CallExtension{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

