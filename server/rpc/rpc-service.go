package rpc

import (
	"context"

	"konjiyami/protobuf/commonpb"
	"konjiyami/protobuf/konjiyamipb"
)

// StartService creates and starts a Windows service on a remote host
func (rpc *Server) StartService(ctx context.Context, req *konjiyamipb.StartServiceReq) (*konjiyamipb.ServiceInfo, error) {
	resp := &konjiyamipb.ServiceInfo{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// StopService stops a remote service
func (rpc *Server) StopService(ctx context.Context, req *konjiyamipb.StopServiceReq) (*konjiyamipb.ServiceInfo, error) {
	resp := &konjiyamipb.ServiceInfo{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// RemoveService deletes a service from the remote system
func (rpc *Server) RemoveService(ctx context.Context, req *konjiyamipb.RemoveServiceReq) (*konjiyamipb.ServiceInfo, error) {
	resp := &konjiyamipb.ServiceInfo{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

