package middleware

import (
	"embed"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	echomd "github.com/labstack/echo/v4/middleware"
	"github.com/valyala/fasttemplate"
	"github.com/ybkuroki/go-webapp-sample/container"
	"gopkg.in/boj/redistore.v1"
)

// InitLoggerMiddleware initialize a middleware for logger.
func InitLoggerMiddleware(e *echo.Echo, container container.Container) {
	e.Use(RequestLoggerMiddleware(container))
	e.Use(ActionLoggerMiddleware(container))
}

// InitSessionMiddleware initialize a middleware for session management.
func InitSessionMiddleware(e *echo.Echo, container container.Container) {
	conf := container.GetConfig()
	logger := container.GetLogger()

	e.Use(SessionMiddleware(container))

	if conf.Extension.SecurityEnabled {
		if conf.Redis.Enabled {
			logger.GetZapLogger().Infof("Try redis connection")
			address := fmt.Sprintf("%s:%s", conf.Redis.Host, conf.Redis.Port)
			store, err := redistore.NewRediStore(conf.Redis.ConnectionPoolSize, "tcp", address, "", []byte("secret"))
			if err != nil {
				logger.GetZapLogger().Errorf("Failure redis connection")
			}
			e.Use(session.Middleware(store))
			logger.GetZapLogger().Infof(fmt.Sprintf("Success redis connection, %s", address))
		} else {
			e.Use(session.Middleware(sessions.NewCookieStore([]byte("secret"))))
		}
		e.Use(AuthenticationMiddleware(container))
	}
}

// RequestLoggerMiddleware is middleware for logging the contents of requests.
func RequestLoggerMiddleware(container container.Container) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			res := c.Response()
			if err := next(c); err != nil {
				c.Error(err)
			}

			template := fasttemplate.New(container.GetConfig().Log.RequestLogFormat, "${", "}")
			logstr := template.ExecuteFuncString(func(w io.Writer, tag string) (int, error) {
				switch tag {
				case "remote_ip":
					return w.Write([]byte(c.RealIP()))
				case "account_name":
					if account := container.GetSession().GetAccount(); account != nil {
						return w.Write([]byte(account.Name))
					}
					return w.Write([]byte("None"))
				case "uri":
					return w.Write([]byte(req.RequestURI))
				case "method":
					return w.Write([]byte(req.Method))
				case "status":
					return w.Write([]byte(strconv.Itoa(res.Status)))
				default:
					return w.Write([]byte(""))
				}
			})
			container.GetLogger().GetZapLogger().Infof(logstr)
			return nil
		}
	}
}

// ActionLoggerMiddleware is middleware for logging the start and end of controller processes.
// ref: https://echo.labstack.com/cookbook/middleware
func ActionLoggerMiddleware(container container.Container) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			logger := container.GetLogger()
			logger.GetZapLogger().Debugf(c.Path() + " Action Start")
			if err := next(c); err != nil {
				c.Error(err)
			}
			logger.GetZapLogger().Debugf(c.Path() + " Action End")
			return nil
		}
	}
}

// SessionMiddleware is a middleware for setting a context to a session.
func SessionMiddleware(container container.Container) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			container.GetSession().SetContext(c)
			if err := next(c); err != nil {
				c.Error(err)
			}
			return nil
		}
	}
}

// StaticContentsMiddleware is the middleware for loading the static files.
func StaticContentsMiddleware(e *echo.Echo, container container.Container, staticFile embed.FS) {
	conf := container.GetConfig()
	if conf.StaticContents.Enabled {
		staticConfig := echomd.StaticConfig{
			Root:       "resources/public",
			Index:      "index.html",
			Browse:     false,
			HTML5:      true,
			Filesystem: http.FS(staticFile),
		}
		if conf.Swagger.Enabled {
			staticConfig.Skipper = func(c echo.Context) bool {
				return equalPath(c.Path(), []string{conf.Swagger.Path})
			}
		}
		e.Use(echomd.StaticWithConfig(staticConfig))
		container.GetLogger().GetZapLogger().Infof("Served the static contents.")
	}
}

// AuthenticationMiddleware is the middleware of session authentication for echo.
func AuthenticationMiddleware(container container.Container) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !hasAuthorization(c, container) {
				return c.JSON(http.StatusUnauthorized, false)
			}
			if err := next(c); err != nil {
				c.Error(err)
			}
			return nil
		}
	}
}

// hasAuthorization judges whether the user has the right to access the path.
func hasAuthorization(c echo.Context, container container.Container) bool {
	currentPath := c.Path()
	if equalPath(currentPath, container.GetConfig().Security.AuthPath) {
		if equalPath(currentPath, container.GetConfig().Security.ExculdePath) {
			return true
		}
		account := container.GetSession().GetAccount()
		if account == nil {
			return false
		}
		if account.Authority.Name == "Admin" && equalPath(currentPath, container.GetConfig().Security.AdminPath) {
			_ = container.GetSession().Save()
			return true
		}
		if account.Authority.Name == "User" && equalPath(currentPath, container.GetConfig().Security.UserPath) {
			_ = container.GetSession().Save()
			return true
		}
		return false
	}
	return true
}

// equalPath judges whether a given path contains in the path list.
func equalPath(cpath string, paths []string) bool {
	for i := range paths {
		if regexp.MustCompile(paths[i]).Match([]byte(cpath)) {
			return true
		}
	}
	return false
}
