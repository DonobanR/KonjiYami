package rpc

import (
	"context"

	"konjiyami/protobuf/commonpb"
	"konjiyami/protobuf/konjiyamipb"
)

// Ping - Try to send a round trip message to the implant
func (rpc *Server) Ping(ctx context.Context, req *konjiyamipb.Ping) (*konjiyamipb.Ping, error) {
	resp := &konjiyamipb.Ping{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

