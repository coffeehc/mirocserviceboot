package base

const (
	ERROR_CODE_BASE_SCOPE = 0x00000000
)

var (
	ERROR_CODE_BASE_SYSTEM_ERROR int64 = ERROR_CODE_BASE_SCOPE | 0x1    //系统错误
	ERROR_CODE_BASE_INVALID_PARAMTER int64 = ERROR_CODE_BASE_SCOPE | 0x2    //无效参数
	ERROR_CODE_BASE_DECODE_ERROR int64 = ERROR_CODE_BASE_SCOPE | 0x3    //解码失败
	ERROR_CODE_BASE_RESPONSE_API_ERROR int64 = ERROR_CODE_BASE_SCOPE | 0x4  //API请求错误
	ERROR_CODE_BASE_404 int64 = ERROR_CODE_BASE_SCOPE | 0x5 //找不到页面或者 Get 时候没有值
	ERROR_CODE_BASE_403 int64 = ERROR_CODE_BASE_SCOPE | 0x6 //没有权限
	ERROR_CODE_BASE_401 int64 = ERROR_CODE_BASE_SCOPE | 0x7 //认证失败
)
