package grpcclient

import (
	"fmt"
	"sync"

	"github.com/coffeehc/logger"
	"github.com/coffeehc/microserviceboot/base"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

var (
	_unaryClientInterceptor = newUnartClientInterceptor()
)

const _internalInvoker = "_internal_invoker"
const context_serviceInfoKey = "__serviceInfo__"

func wapperUnartClientInterceptor(serviceInfo base.ServiceInfo) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) (err error) {
		return _unaryClientInterceptor.Interceptor(context.WithValue(ctx, context_serviceInfoKey, serviceInfo), method, req, reply, cc, invoker, opts...)
	}
}

//AppendUnartClientInterceptor 追加一个UnartClientInterceptor
func AppendUnartClientInterceptor(name string, unaryClientInterceptor grpc.UnaryClientInterceptor) base.Error {
	return _unaryClientInterceptor.AppendInterceptor(name, unaryClientInterceptor)
}

func newUnartClientInterceptor() *unartClientInterceptor {
	return &unartClientInterceptor{
		interceptors: make(map[string]*unaryClientInterceptorWapper),
		rootInterceptor: &unaryClientInterceptorWapper{
			interceptor: paincInterceptor,
		},
		mutex: new(sync.Mutex),
	}
}

type unartClientInterceptor struct {
	interceptors    map[string]*unaryClientInterceptorWapper
	rootInterceptor *unaryClientInterceptorWapper
	mutex           *sync.Mutex
}

func (uci *unartClientInterceptor) Interceptor(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) (err error) {
	opts = append(opts, grpc.FailFast(false))
	return uci.rootInterceptor.interceptor(context.WithValue(ctx, _internalInvoker, invoker), method, req, reply, cc, uci.rootInterceptor.invoker, opts...)
}

func (uci *unartClientInterceptor) AppendInterceptor(name string, interceptor grpc.UnaryClientInterceptor) base.Error {
	uci.mutex.Lock()
	defer uci.mutex.Unlock()
	if _, ok := uci.interceptors[name]; ok {
		return base.NewError(base.Error_System, "grpc interceptor", fmt.Sprintf("%s 已经存在", name))
	}
	lastInterceptor := getLastUnaryClientInterceptor(uci.rootInterceptor)
	lastInterceptor.next = &unaryClientInterceptorWapper{interceptor: interceptor}
	uci.interceptors[name] = lastInterceptor.next
	return nil
}

func getLastUnaryClientInterceptor(root *unaryClientInterceptorWapper) *unaryClientInterceptorWapper {
	if root.next == nil {
		return root
	}
	return getLastUnaryClientInterceptor(root.next)
}

type unaryClientInterceptorWapper struct {
	interceptor grpc.UnaryClientInterceptor
	next        *unaryClientInterceptorWapper
}

func (uciw *unaryClientInterceptorWapper) invoker(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) (err error) {
	if uciw.next == nil {
		realInvoker := ctx.Value(_internalInvoker)
		if realInvoker == nil {
			return base.NewError(base.Error_System, "grpc", "没有 Handler")
		}
		if invoker, ok := realInvoker.(grpc.UnaryInvoker); ok {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		return base.NewError(base.Error_System, "grpc", "类型错误")
	}
	return uciw.next.interceptor(ctx, method, req, reply, cc, uciw.next.invoker, opts...)
}

func paincInterceptor(cxt context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = adapteError(cxt, r)
		}
	}()
	return adapteError(cxt, invoker(cxt, method, req, reply, cc, opts...))
}

func adapteError(cxt context.Context, err interface{}) base.Error {
	if err == nil {
		return nil
	}
	serviceName := "未知服务"
	serviceInfo, ok := cxt.Value(context_serviceInfoKey).(base.ServiceInfo)
	if ok {
		serviceName = serviceInfo.GetServiceName()
	}
	serviceName = "grpc:" + serviceName
	if base.IsDevModule() {
		logger.Error("发生异常:%#v", err)
	}
	switch v := err.(type) {
	case base.Error:
		return v
	case string:
		return base.NewError(base.Error_System, serviceName, v)
	case error:
		if s, ok := status.FromError(v); ok {
			code := int32(s.Code())
			if !base.IsBaseErrorCode(code) {
				return base.NewErrorWrapper(base.Error_System, serviceName, s.Err())
			}
			return base.NewError(code, serviceName, s.Message())
		}
		return base.NewErrorWrapper(base.Error_System, serviceName, v)
	default:
		return base.NewError(base.Error_System, serviceName, fmt.Sprintf("未知异常:%#v", v))
	}

}
