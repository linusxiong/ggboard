package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"
)

var setupOnce sync.Once

// setup configures gin globals once: release mode and JSON field names in validation errors.
func setup() {
	setupOnce.Do(func() {
		gin.SetMode(gin.ReleaseMode)
		if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
			v.RegisterTagNameFunc(func(f reflect.StructField) string {
				for _, tag := range []string{"json", "form", "uri"} {
					name, _, _ := strings.Cut(f.Tag.Get(tag), ",")
					if name == "-" {
						return ""
					}
					if name != "" {
						return name
					}
				}
				return f.Name
			})
		}
	})
}

// Validate runs the gin validator on v (a struct pointer) and returns a Validation error.
func Validate(v any) error {
	setup()
	if err := binding.Validator.ValidateStruct(v); err != nil {
		return Validation(err)
	}
	return nil
}

// Bind decodes the request into req and validates it (binding:"..." tags). GET/HEAD/DELETE bind the query
// string; other methods bind by Content-Type (JSON by default; an empty body binds nothing and is then
// validated). Errors are returned as *Error (400/413) ready for Fail.
func Bind(c *gin.Context, req any) error {
	setup()
	var err error
	switch c.Request.Method {
	case http.MethodGet, http.MethodHead, http.MethodDelete:
		err = c.ShouldBindQuery(req)
	default:
		switch c.ContentType() {
		case "", binding.MIMEJSON:
			err = c.ShouldBindJSON(req)
			if errors.Is(err, io.EOF) {
				err = binding.Validator.ValidateStruct(req)
			}
		default:
			err = c.ShouldBind(req)
		}
	}
	if err != nil {
		return Validation(err)
	}
	return nil
}

// BindQuery binds and validates the query string into req.
func BindQuery(c *gin.Context, req any) error {
	setup()
	if err := c.ShouldBindQuery(req); err != nil {
		return Validation(err)
	}
	return nil
}

// ParamID parses the path parameter name as a positive uint64 (404 when invalid).
func ParamID(c *gin.Context, name string) (uint64, error) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		return 0, NotFound("resource not found")
	}
	return id, nil
}

// Validation translates binding/validation errors into a 400 validation_failed *Error whose Errors map uses
// JSON field names (nested: "items[0].name").
func Validation(err error) *Error {
	if err == nil {
		return nil
	}
	if e := fromKnown(err); e != nil {
		return e
	}
	var he *Error
	if errors.As(err, &he) {
		return he
	}
	return BadRequest("invalid request").WithCause(err)
}

func fromKnown(err error) *Error {
	var (
		ves  validator.ValidationErrors
		ute  *json.UnmarshalTypeError
		se   *json.SyntaxError
		mbe  *http.MaxBytesError
		num  *strconv.NumError
		inve *validator.InvalidValidationError
	)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return NotFound("resource not found")
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return Conflict("resource already exists")
	case errors.As(err, &mbe):
		return PayloadTooLarge(fmt.Sprintf("request body exceeds %d bytes", mbe.Limit))
	case errors.As(err, &ves):
		e := BadRequest("validation failed")
		for _, fe := range ves {
			e = e.WithField(fieldPath(fe), message(fe))
		}
		return e
	case errors.As(err, &ute):
		field := ute.Field
		if field == "" {
			return BadRequest("invalid JSON body").WithCause(err)
		}
		return BadRequest("validation failed").WithField(field, "must be "+kindName(ute.Type)).WithCause(err)
	case errors.As(err, &se), errors.Is(err, io.ErrUnexpectedEOF):
		return BadRequest("invalid JSON body").WithCause(err)
	case errors.As(err, &num):
		return BadRequest("invalid parameter value").WithCause(err)
	case errors.As(err, &inve):
		return Internal(err)
	}
	return nil
}

// fieldPath strips the root struct name from the validator namespace ("Req.items[0].name" -> "items[0].name").
func fieldPath(fe validator.FieldError) string {
	ns := fe.Namespace()
	if _, rest, ok := strings.Cut(ns, "."); ok {
		return rest
	}
	return fe.Field()
}

func kindName(t reflect.Type) string {
	if t == nil {
		return "a valid value"
	}
	switch t.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "a boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "an integer"
	case reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.Slice, reflect.Array:
		return "an array"
	case reflect.Map, reflect.Struct:
		return "an object"
	}
	return "a valid value"
}

func message(fe validator.FieldError) string {
	p := fe.Param()
	kind := fe.Kind()
	isLen := kind == reflect.String || kind == reflect.Slice || kind == reflect.Map || kind == reflect.Array
	unit := "characters"
	if kind != reflect.String {
		unit = "items"
	}
	switch fe.Tag() {
	case "required", "required_if", "required_unless", "required_with", "required_without", "required_with_all", "required_without_all":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "url", "http_url", "uri":
		return "must be a valid URL"
	case "uuid", "uuid4", "uuid_rfc4122":
		return "must be a valid UUID"
	case "min":
		if isLen {
			return fmt.Sprintf("must be at least %s %s", p, unit)
		}
		return "must be at least " + p
	case "max":
		if isLen {
			return fmt.Sprintf("must be at most %s %s", p, unit)
		}
		return "must be at most " + p
	case "len":
		if isLen {
			return fmt.Sprintf("must be exactly %s %s", p, unit)
		}
		return "must be " + p
	case "gte":
		return "must be greater than or equal to " + p
	case "gt":
		return "must be greater than " + p
	case "lte":
		return "must be less than or equal to " + p
	case "lt":
		return "must be less than " + p
	case "eq":
		return "must be " + p
	case "ne":
		return "must not be " + p
	case "oneof":
		return "must be one of: " + strings.Join(strings.Fields(p), ", ")
	case "alphanum":
		return "must contain only letters and digits"
	case "alpha":
		return "must contain only letters"
	case "numeric", "number":
		return "must be numeric"
	case "ip", "ipv4", "ipv6":
		return "must be a valid IP address"
	case "cidr":
		return "must be a valid CIDR"
	case "hostname", "hostname_rfc1123", "fqdn":
		return "must be a valid hostname"
	case "hexcolor":
		return "must be a hex color"
	case "datetime":
		return "must be a date/time in format " + p
	case "eqfield":
		return "must match " + p
	case "nefield":
		return "must differ from " + p
	case "unique":
		return "must not contain duplicates"
	case "excludesall", "excludes":
		return "contains forbidden characters"
	}
	return "is invalid (" + fe.Tag() + ")"
}
