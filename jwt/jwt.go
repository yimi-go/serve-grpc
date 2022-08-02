package jwt

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/yimi-go/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	authorizationKey      = "Authorization"
	bearerWord            = "Bearer"
	reason                = "UNAUTHORIZED"
	reasonNoAuthHead      = "missing_authorization_head"
	reasonInvalidAuthHead = "invalid_auth_head"
)

var (
	errMissingAuthHead = errors.Unauthenticated(reasonNoAuthHead, "missing authorization header")
	errInvalidAuthHead = errors.Unauthenticated(reasonInvalidAuthHead, "invalid authorization header")
	errTokenInvalid    = errors.Unauthenticated(reason, "invalid authorization token")
	errTokenExpired    = errors.Unauthenticated(reason, "authorization is expired")
)

type options struct {
	signingMethod []jwt.SigningMethod
	claims        func() jwt.Claims
}

func (o *options) methods() []string {
	methods := make([]string, 0, len(o.signingMethod))
	for _, method := range o.signingMethod {
		methods = append(methods, method.Alg())
	}
	return methods
}

type Option func(*options)

func WithSigningMethod(method ...jwt.SigningMethod) Option {
	return func(o *options) {
		o.signingMethod = method
	}
}

func WithClaims(f func() jwt.Claims) Option {
	return func(o *options) {
		o.claims = f
	}
}

func parseClaimsCtx(ctx context.Context, o *options, keyFunc jwt.Keyfunc) (context.Context, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	authHeadValues := md.Get(authorizationKey)
	if len(authHeadValues) != 1 {
		return ctx, errMissingAuthHead
	}
	auths := strings.SplitN(authHeadValues[0], " ", 2)
	if len(auths) != 2 || !strings.EqualFold(auths[0], bearerWord) {
		return ctx, errInvalidAuthHead
	}
	jwtToken := auths[1]
	var (
		tokenInfo *jwt.Token
		err       error
	)
	if o.claims != nil {
		tokenInfo, err = jwt.ParseWithClaims(jwtToken, o.claims(), keyFunc, jwt.WithValidMethods(o.methods()))
	} else {
		tokenInfo, err = jwt.Parse(jwtToken, keyFunc, jwt.WithValidMethods(o.methods()))
	}
	if err != nil {
		ve, ok := err.(*jwt.ValidationError)
		if !ok {
			// imposable.
			// For golang-jwt/jwt/v4@v4.4.2, Parse and ParseWithClaims only produces *jwt.ValidationError
			return nil, errors.Unauthenticated(reason, err.Error()).WithCause(err)
		}
		if ve.Errors&(jwt.ValidationErrorExpired|jwt.ValidationErrorNotValidYet) != 0 {
			return ctx, errTokenExpired.WithCause(err)
		}
		return ctx, errTokenInvalid.WithCause(err)
	}
	ctx = NewContext(ctx, tokenInfo.Claims)
	return ctx, nil
}

func UnaryInterceptor(keyFunc jwt.Keyfunc, opts ...Option) (grpc.UnaryServerInterceptor, error) {
	if keyFunc == nil {
		return nil, fmt.Errorf("grpc: jwt: a jwt.keyfunc is required")
	}
	o := &options{
		signingMethod: []jwt.SigningMethod{jwt.SigningMethodHS256},
	}
	for _, opt := range opts {
		opt(o)
	}
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		ctx, err = parseClaimsCtx(ctx, o, keyFunc)
		if err != nil {
			return
		}
		return handler(ctx, req)
	}, nil
}

func StreamInterceptor(keyFunc jwt.Keyfunc, opts ...Option) (grpc.StreamClientInterceptor, error) {
	if keyFunc == nil {
		return nil, fmt.Errorf("grpc: jwt: a jwt.keyfunc is required")
	}
	o := &options{
		signingMethod: []jwt.SigningMethod{jwt.SigningMethodHS256},
	}
	for _, opt := range opts {
		opt(o)
	}
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx, err := parseClaimsCtx(ctx, o, keyFunc)
		if err != nil {
			return nil, err
		}
		return streamer(ctx, desc, cc, method, opts...)
	}, nil
}

type claimsKey struct{}

func NewContext(ctx context.Context, claims jwt.Claims) context.Context {
	return context.WithValue(ctx, claimsKey{}, claims)
}

func FromContext(ctx context.Context) (claims jwt.Claims, ok bool) {
	claims, ok = ctx.Value(claimsKey{}).(jwt.Claims)
	return
}
