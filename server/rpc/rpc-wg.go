package rpc

import (
	"context"

	"konjiyami/protobuf/clientpb"
	"konjiyami/protobuf/commonpb"
	"konjiyami/protobuf/konjiyamipb"
	"konjiyami/server/certs"
	"konjiyami/server/generate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GenerateWGClientConfig - Generate a client config for a WG interface
func (rpc *Server) GenerateWGClientConfig(ctx context.Context, _ *commonpb.Empty) (*clientpb.WGClientConfig, error) {
	clientIP, err := generate.GenerateUniqueIP()
	if err != nil {
		rpcLog.Errorf("Could not generate WG unique IP: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	privkey, pubkey, err := certs.GenerateWGKeys(true, clientIP.String())
	if err != nil {
		rpcLog.Errorf("Could not generate WG keys: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	_, serverPubKey, err := certs.GetWGServerKeys()
	if err != nil {
		rpcLog.Errorf("Could not get WG server keys: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	resp := &clientpb.WGClientConfig{
		ClientPrivateKey: privkey,
		ClientIP:         clientIP.String(),
		ClientPubKey:     pubkey,
		ServerPubKey:     serverPubKey,
	}

	return resp, nil
}

// WGStartPortForward - Start a port forward
func (rpc *Server) WGStartPortForward(ctx context.Context, req *konjiyamipb.WGPortForwardStartReq) (*konjiyamipb.WGPortForward, error) {
	resp := &konjiyamipb.WGPortForward{}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// WGStopPortForward - Stop a port forward
func (rpc *Server) WGStopPortForward(ctx context.Context, req *konjiyamipb.WGPortForwardStopReq) (*konjiyamipb.WGPortForward, error) {
	resp := &konjiyamipb.WGPortForward{}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// WGAddForwarder - Add a TCP forwarder
func (rpc *Server) WGStartSocks(ctx context.Context, req *konjiyamipb.WGSocksStartReq) (*konjiyamipb.WGSocks, error) {
	resp := &konjiyamipb.WGSocks{}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// WGStopForwarder - Stop a TCP forwarder
func (rpc *Server) WGStopSocks(ctx context.Context, req *konjiyamipb.WGSocksStopReq) (*konjiyamipb.WGSocks, error) {
	resp := &konjiyamipb.WGSocks{}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (rpc *Server) WGListSocksServers(ctx context.Context, req *konjiyamipb.WGSocksServersReq) (*konjiyamipb.WGSocksServers, error) {
	resp := &konjiyamipb.WGSocksServers{}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// WGAddForwarder - List wireguard forwarders
func (rpc *Server) WGListForwarders(ctx context.Context, req *konjiyamipb.WGTCPForwardersReq) (*konjiyamipb.WGTCPForwarders, error) {
	resp := &konjiyamipb.WGTCPForwarders{}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

