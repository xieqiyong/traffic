package proto

import (
	"bytes"
	hessian "github.com/apache/dubbo-go-hessian2"
	perrors "github.com/pkg/errors"
)
import (
	"dubbo.apache.org/dubbo-go/v3/common/constant"
	"dubbo.apache.org/dubbo-go/v3/common/logger"
	"dubbo.apache.org/dubbo-go/v3/protocol"
	"dubbo.apache.org/dubbo-go/v3/protocol/dubbo/impl"
	"dubbo.apache.org/dubbo-go/v3/protocol/invocation"
	"dubbo.apache.org/dubbo-go/v3/remoting"
)



// SerialID serial ID
type SerialID byte

type DubboCodec struct{}

// Decode data, including request and response.
func (c *DubboCodec) Decode(data []byte) (remoting.DecodeResult, int, error) {
	if c.isRequest(data) {
		req, len, err := c.decodeRequest(data)
		if err != nil {
			return remoting.DecodeResult{}, len, perrors.WithStack(err)
		}
		return remoting.DecodeResult{IsRequest: true, Result: req}, len, perrors.WithStack(err)
	}

	resp, len, err := c.decodeResponse(data)
	if err != nil {
		return remoting.DecodeResult{}, len, perrors.WithStack(err)
	}
	return remoting.DecodeResult{IsRequest: false, Result: resp}, len, perrors.WithStack(err)
}

func (c *DubboCodec) isRequest(data []byte) bool {
	return data[2]&byte(0x80) != 0x00
}

// decode request
func (c *DubboCodec) decodeRequest(data []byte) (*remoting.Request, int, error) {
	var request *remoting.Request = nil
	buf := bytes.NewBuffer(data)
	pkg := impl.NewDubboPackage(buf)
	pkg.SetBody(make([]interface{}, 7))
	err := pkg.Unmarshal()
	if err != nil {
		originErr := perrors.Cause(err)
		if originErr == hessian.ErrHeaderNotEnough || originErr == hessian.ErrBodyNotEnough {
			// FIXME
			return nil, 0, originErr
		}
		logger.Errorf("pkg.Unmarshal(len(@data):%d) = error:%+v", buf.Len(), err)

		return request, 0, perrors.WithStack(err)
	}
	request = &remoting.Request{
		ID:       pkg.Header.ID,
		SerialID: pkg.Header.SerialID,
		TwoWay:   pkg.Header.Type&impl.PackageRequest_TwoWay != 0x00,
		Event:    pkg.Header.Type&impl.PackageHeartbeat != 0x00,
	}
	if (pkg.Header.Type & impl.PackageHeartbeat) == 0x00 {
		// convert params of request
		req := pkg.Body.(map[string]interface{})

		// invocation := request.Data.(*invocation.RPCInvocation)
		var methodName string
		var args []interface{}
		attachments := make(map[string]interface{})
		if req[impl.DubboVersionKey] != nil {
			// dubbo version
			request.Version = req[impl.DubboVersionKey].(string)
		}
		// path
		attachments[constant.PathKey] = pkg.Service.Path
		// version
		attachments[constant.VersionKey] = pkg.Service.Version
		// method
		methodName = pkg.Service.Method
		args = req[impl.ArgsKey].([]interface{})
		attachments = req[impl.AttachmentsKey].(map[string]interface{})
		invoc := invocation.NewRPCInvocationWithOptions(invocation.WithAttachments(attachments),
			invocation.WithArguments(args), invocation.WithMethodName(methodName))
		request.Data = invoc

	}
	return request, hessian.HEADER_LENGTH + pkg.Header.BodyLen, nil
}

// decode response
func (c *DubboCodec) decodeResponse(data []byte) (*remoting.Response, int, error) {
	buf := bytes.NewBuffer(data)
	pkg := impl.NewDubboPackage(buf)
	err := pkg.Unmarshal()
	if err != nil {
		originErr := perrors.Cause(err)
		// if the data is very big, so the receive need much times.
		if originErr == hessian.ErrHeaderNotEnough || originErr == hessian.ErrBodyNotEnough {
			return nil, 0, originErr
		}
		logger.Errorf("pkg.Unmarshal(len(@data):%d) = error:%+v", buf.Len(), err)

		return nil, 0, perrors.WithStack(err)
	}
	response := &remoting.Response{
		ID: pkg.Header.ID,
		// Version:  pkg.Header.,
		SerialID: pkg.Header.SerialID,
		Status:   pkg.Header.ResponseStatus,
		Event:    (pkg.Header.Type & impl.PackageHeartbeat) != 0,
	}
	var pkgerr error
	if pkg.Header.Type&impl.PackageHeartbeat != 0x00 {
		if pkg.Header.Type&impl.PackageResponse != 0x00 {
			logger.Debugf("get rpc heartbeat response{header: %#v, body: %#v}", pkg.Header, pkg.Body)
			if pkg.Err != nil {
				logger.Errorf("rpc heartbeat response{error: %#v}", pkg.Err)
				pkgerr = pkg.Err
			}
		} else {
			logger.Debugf("get rpc heartbeat request{header: %#v, service: %#v, body: %#v}", pkg.Header, pkg.Service, pkg.Body)
			response.Status = hessian.Response_OK
			// reply(session, p, hessian.PackageHeartbeat)
		}
		return response, hessian.HEADER_LENGTH + pkg.Header.BodyLen, pkgerr
	}
	logger.Debugf("get rpc response{header: %#v, body: %#v}", pkg.Header, pkg.Body)
	rpcResult := &protocol.RPCResult{}
	response.Result = rpcResult
	if pkg.Header.Type&impl.PackageRequest == 0x00 {
		if pkg.Err != nil {
			rpcResult.Err = pkg.Err
		} else if pkg.Body.(*impl.ResponsePayload).Exception != nil {
			rpcResult.Err = pkg.Body.(*impl.ResponsePayload).Exception
			response.Error = rpcResult.Err
		}
		rpcResult.Attrs = pkg.Body.(*impl.ResponsePayload).Attachments
		rpcResult.Rest = pkg.Body.(*impl.ResponsePayload).RspObj
	}

	return response, hessian.HEADER_LENGTH + pkg.Header.BodyLen, nil
}
