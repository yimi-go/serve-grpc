package jwt

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type testClaims struct {
	Foo string `json:"foo"`
	jwt.RegisteredClaims
	validFn func() error
}

func (t *testClaims) Valid() error {
	err := t.RegisteredClaims.Valid()
	if err != nil {
		return err
	}
	if t.validFn == nil {
		return nil
	}
	return t.validFn()
}

func Test_options_methods(t *testing.T) {
	type fields struct {
		signingMethod []jwt.SigningMethod
	}
	tests := []struct {
		name   string
		fields fields
		want   []string
	}{
		{
			name:   "nil",
			fields: fields{signingMethod: nil},
			want:   []string{},
		},
		{
			name:   "blank",
			fields: fields{signingMethod: []jwt.SigningMethod{}},
			want:   []string{},
		},
		{
			name: jwt.SigningMethodHS256.Alg(),
			fields: fields{
				signingMethod: []jwt.SigningMethod{
					jwt.SigningMethodHS256,
				},
			},
			want: []string{
				jwt.SigningMethodHS256.Alg(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &options{
				signingMethod: tt.fields.signingMethod,
			}
			assert.Equal(t, tt.want, o.methods())
		})
	}
}

func TestWithSigningMethod(t *testing.T) {
	o := &options{}
	m := jwt.SigningMethodHS256
	WithSigningMethod(m)(o)
	assert.Equal(t, []jwt.SigningMethod{m}, o.signingMethod)
}

func TestWithClaims(t *testing.T) {
	o := &options{}
	c := jwt.MapClaims{}
	WithClaims(func() jwt.Claims {
		return c
	})(o)
	assert.Equal(t, c, o.claims())
}

func Test_parseClaimsCtx(t *testing.T) {
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		return []byte("AllYourBase"), nil
	}
	validTokenString := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJmb28iOiJiYXIiLCJpc3MiOiJ0ZXN0IiwiYXVkIjoic2luZ2xlIn0." +
		"QAWg1vGvnqRuCFTMcPkjZljXHh8U3L_qUjszOtQbeaA"
	validMd := metadata.MD{
		authorizationKey: []string{fmt.Sprintf("%s %s", bearerWord, validTokenString)},
	}
	validCtx := metadata.NewIncomingContext(context.Background(), validMd)
	t.Run("missing_auth_head", func(t *testing.T) {
		ctx, err := parseClaimsCtx(context.Background(), &options{}, keyFunc)
		assert.NotNil(t, err)
		assert.Same(t, context.Background(), ctx)
	})
	t.Run("no_bearer_word", func(t *testing.T) {
		md := metadata.MD{
			authorizationKey: []string{validTokenString},
		}
		ctxWithMd := metadata.NewIncomingContext(context.Background(), md)
		ctx, err := parseClaimsCtx(ctxWithMd, &options{}, keyFunc)
		assert.NotNil(t, err)
		assert.Same(t, ctxWithMd, ctx)
	})
	t.Run("not_bearer_word", func(t *testing.T) {
		md := metadata.MD{
			authorizationKey: []string{fmt.Sprintf("%s %s", "basic", validTokenString)},
		}
		ctxWithMd := metadata.NewIncomingContext(context.Background(), md)
		ctx, err := parseClaimsCtx(ctxWithMd, &options{}, keyFunc)
		assert.NotNil(t, err)
		assert.Same(t, ctxWithMd, ctx)
	})
	t.Run("with_testClaims_expired", func(t *testing.T) {
		o := &options{
			signingMethod: []jwt.SigningMethod{jwt.SigningMethodHS256},
			claims: func() jwt.Claims {
				return &testClaims{
					validFn: func() error {
						return jwt.NewValidationError("test", jwt.ValidationErrorExpired)
					},
				}
			},
		}
		ctx, err := parseClaimsCtx(validCtx, o, keyFunc)
		assert.NotNil(t, err)
		assert.Same(t, validCtx, ctx)
	})
	t.Run("with_testClaims_not_valid_yet", func(t *testing.T) {
		o := &options{
			signingMethod: []jwt.SigningMethod{jwt.SigningMethodHS256},
			claims: func() jwt.Claims {
				return &testClaims{
					validFn: func() error {
						return jwt.NewValidationError("test", jwt.ValidationErrorNotValidYet)
					},
				}
			},
		}
		ctx, err := parseClaimsCtx(validCtx, o, keyFunc)
		assert.NotNil(t, err)
		assert.Same(t, validCtx, ctx)
	})
	t.Run("with_testClaims_invalid", func(t *testing.T) {
		o := &options{
			signingMethod: []jwt.SigningMethod{jwt.SigningMethodHS256},
			claims: func() jwt.Claims {
				return &testClaims{
					validFn: func() error {
						return jwt.NewValidationError("test", jwt.ValidationErrorMalformed)
					},
				}
			},
		}
		ctx, err := parseClaimsCtx(validCtx, o, keyFunc)
		assert.NotNil(t, err)
		assert.Same(t, validCtx, ctx)
	})
	t.Run("with_testClaims_ok", func(t *testing.T) {
		o := &options{
			signingMethod: []jwt.SigningMethod{jwt.SigningMethodHS256},
			claims: func() jwt.Claims {
				return &testClaims{}
			},
		}
		ctx, err := parseClaimsCtx(validCtx, o, keyFunc)
		assert.Nil(t, err)
		assert.NotSame(t, validCtx, ctx)
		claims, ok := FromContext(ctx)
		assert.True(t, ok)
		assert.NotNil(t, claims)
		assert.IsType(t, &testClaims{}, claims)
		tc := claims.(*testClaims)
		assert.Equal(t, "bar", tc.Foo)
	})
	t.Run("invalid", func(t *testing.T) {
		o := &options{
			signingMethod: []jwt.SigningMethod{jwt.SigningMethodHS256},
		}
		ctx, err := parseClaimsCtx(validCtx, o, func(token *jwt.Token) (interface{}, error) {
			return []byte("notvalidkey"), nil
		})
		assert.NotNil(t, err)
		assert.Same(t, validCtx, ctx)
	})
	t.Run("ok", func(t *testing.T) {
		o := &options{
			signingMethod: []jwt.SigningMethod{jwt.SigningMethodHS256},
		}
		ctx, err := parseClaimsCtx(validCtx, o, keyFunc)
		assert.Nil(t, err)
		assert.NotSame(t, validCtx, ctx)
		claims, ok := FromContext(ctx)
		assert.True(t, ok)
		assert.NotNil(t, claims)
	})
}

func TestUnaryInterceptor(t *testing.T) {
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		return []byte("AllYourBase"), nil
	}
	tokenString := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJmb28iOiJiYXIiLCJpc3MiOiJ0ZXN0IiwiYXVkIjoic2luZ2xlIn0." +
		"QAWg1vGvnqRuCFTMcPkjZljXHh8U3L_qUjszOtQbeaA"
	md := metadata.MD{
		authorizationKey: []string{fmt.Sprintf("%s %s", bearerWord, tokenString)},
	}
	ctx := metadata.NewIncomingContext(context.Background(), md)
	info := &grpc.UnaryServerInfo{FullMethod: "testMethod"}
	t.Run("keyFunc_missing", func(t *testing.T) {
		_, err := UnaryInterceptor(nil)
		assert.NotNil(t, err)
	})
	t.Run("opts", func(t *testing.T) {
		count := 0
		_, _ = UnaryInterceptor(keyFunc, func(o *options) {
			count++
		})
		assert.Equal(t, 1, count)
	})
	t.Run("err", func(t *testing.T) {
		interceptor, err := UnaryInterceptor(keyFunc)
		assert.Nil(t, err)
		count := 0
		_, err = interceptor(context.Background(), "hello", info, func(ctx context.Context, req any) (any, error) {
			count++
			return nil, nil
		})
		assert.NotNil(t, err)
		assert.Equal(t, 0, count)
	})
	t.Run("ok", func(t *testing.T) {
		interceptor, err := UnaryInterceptor(keyFunc, WithClaims(func() jwt.Claims {
			return &testClaims{}
		}))
		assert.Nil(t, err)
		count := 0
		resp, err := interceptor(ctx, "hello", info, func(ctx context.Context, req any) (any, error) {
			claims, ok := FromContext(ctx)
			if !ok {
				return nil, fmt.Errorf("can not get claims from context")
			}
			tc := claims.(*testClaims)
			count++
			return tc.Foo, nil
		})
		assert.Nil(t, err)
		assert.Equal(t, 1, count)
		assert.Equal(t, "bar", resp)
	})
}

func TestStreamInterceptor(t *testing.T) {
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		return []byte("AllYourBase"), nil
	}
	tokenString := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJmb28iOiJiYXIiLCJpc3MiOiJ0ZXN0IiwiYXVkIjoic2luZ2xlIn0." +
		"QAWg1vGvnqRuCFTMcPkjZljXHh8U3L_qUjszOtQbeaA"
	md := metadata.MD{
		authorizationKey: []string{fmt.Sprintf("%s %s", bearerWord, tokenString)},
	}
	ctx := metadata.NewIncomingContext(context.Background(), md)
	t.Run("keyFunc_missing", func(t *testing.T) {
		_, err := StreamInterceptor(nil)
		assert.NotNil(t, err)
	})
	t.Run("opts", func(t *testing.T) {
		count := 0
		_, _ = StreamInterceptor(keyFunc, func(o *options) {
			count++
		})
		assert.Equal(t, 1, count)
	})
	t.Run("err", func(t *testing.T) {
		interceptor, err := StreamInterceptor(keyFunc)
		assert.Nil(t, err)
		count := 0
		_, err = interceptor(
			context.Background(),
			&grpc.StreamDesc{},
			&grpc.ClientConn{},
			"testMethod",
			func(
				ctx context.Context,
				desc *grpc.StreamDesc,
				cc *grpc.ClientConn,
				method string,
				opts ...grpc.CallOption,
			) (grpc.ClientStream, error) {
				count++
				return cc.NewStream(ctx, desc, method, opts...)
			})
		assert.NotNil(t, err)
		assert.Equal(t, 0, count)
	})
	t.Run("ok", func(t *testing.T) {
		interceptor, err := StreamInterceptor(keyFunc, WithClaims(func() jwt.Claims {
			return &testClaims{}
		}))
		assert.Nil(t, err)
		count := 0
		_, err = interceptor(
			ctx,
			&grpc.StreamDesc{},
			&grpc.ClientConn{},
			"testMethod",
			func(
				ctx context.Context,
				desc *grpc.StreamDesc,
				cc *grpc.ClientConn,
				method string,
				opts ...grpc.CallOption,
			) (grpc.ClientStream, error) {
				claims, ok := FromContext(ctx)
				assert.True(t, ok, "can not get claims from context")
				tc := claims.(*testClaims)
				assert.Equal(t, "bar", tc.Foo)
				count++
				return nil, nil
			})
		assert.Nil(t, err)
		assert.Equal(t, 1, count)
	})
}
