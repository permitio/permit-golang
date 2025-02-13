package log

const (
	UserKey         = "user"
	ResponseBodyKey = "response_body"
	CountKey        = "count"
)

type Logger interface {
	Debug(msg string, params ...interface{})
	Info(msg string, params ...interface{})
	Warn(msg string, params ...interface{})
	Error(msg string, err error, params ...interface{})
}
