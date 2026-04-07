package errors

import (
	"bytes"
	errorspkg "errors"
	"log"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func resetHTTPOverrides() {
	httpStatusMu.Lock()
	defer httpStatusMu.Unlock()
	httpStatusOverrides = make(map[ErrorCode]int)
	httpStatusResolverFn = nil
	internalLogger = nil
}

func TestBizErrorIsAndUnwrap(t *testing.T) {
	cause := errorspkg.New("root")
	err := Wrap(ErrCodeNotFound, "missing", cause)

	if !Is(err, ErrNotFound) {
		t.Fatalf("expected Is to match ErrNotFound")
	}
	if !errorspkg.Is(err, cause) {
		t.Fatalf("expected errors.Is to match cause")
	}
}

func TestToGRPCError(t *testing.T) {
	err := New(ErrCodeInvalidArgument, "bad")
	grpcErr := ToGRPCError(err)
	st, ok := status.FromError(grpcErr)
	if !ok {
		t.Fatalf("expected grpc status")
	}
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("unexpected grpc code: %v", st.Code())
	}
}

func TestToGRPCErrorMasksNonBizError(t *testing.T) {
	resetHTTPOverrides()
	defer resetHTTPOverrides()

	var gotMsg string
	var gotErr error
	SetInternalLogger(func(msg string, err error) {
		gotMsg = msg
		gotErr = err
	})

	grpcErr := ToGRPCError(errorspkg.New("db password leaked"))
	st, ok := status.FromError(grpcErr)
	if !ok {
		t.Fatalf("expected grpc status")
	}
	if st.Code() != codes.Internal {
		t.Fatalf("unexpected grpc code: %v", st.Code())
	}
	if st.Message() != "internal server error" {
		t.Fatalf("unexpected grpc message: %q", st.Message())
	}
	if gotMsg != "ToGRPCError: non-BizError discarded in response" {
		t.Fatalf("unexpected log message: %q", gotMsg)
	}
	if gotErr == nil || gotErr.Error() != "db password leaked" {
		t.Fatalf("unexpected logged error: %v", gotErr)
	}
}

func TestFromGRPCError(t *testing.T) {
	grpcErr := status.Error(codes.NotFound, "missing")
	bizErr := FromGRPCError(grpcErr)
	if bizErr == nil {
		t.Fatalf("expected biz error")
	}
	if bizErr.Code != ErrCodeNotFound {
		t.Fatalf("unexpected code: %v", bizErr.Code)
	}
	if bizErr.Message != "missing" {
		t.Fatalf("unexpected message: %q", bizErr.Message)
	}
}

func TestToHTTPResponse(t *testing.T) {
	resetHTTPOverrides()
	defer resetHTTPOverrides()

	statusCode, body := ToHTTPResponse(nil)
	if statusCode != 200 {
		t.Fatalf("unexpected status for nil error: %d", statusCode)
	}
	if body["code"].(int) != 0 {
		t.Fatalf("unexpected code for nil error: %v", body["code"])
	}

	RegisterHTTPStatus(ErrCodeNotFound, 410)
	statusCode, _ = ToHTTPResponse(New(ErrCodeNotFound, "gone"))
	if statusCode != 410 {
		t.Fatalf("expected override status, got: %d", statusCode)
	}

	resetHTTPOverrides()
	SetHTTPStatusResolver(func(code ErrorCode) (int, bool) {
		if code == ErrCodePermissionDenied {
			return 451, true
		}
		return 0, false
	})
	statusCode, _ = ToHTTPResponse(New(ErrCodePermissionDenied, "deny"))
	if statusCode != 451 {
		t.Fatalf("expected resolver status, got: %d", statusCode)
	}
}

func TestToHTTPResponseFallsBackToStdLogger(t *testing.T) {
	resetHTTPOverrides()
	defer resetHTTPOverrides()

	var buf bytes.Buffer
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	oldPrefix := log.Prefix()
	log.SetOutput(&buf)
	log.SetFlags(0)
	log.SetPrefix("")
	defer func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	}()

	statusCode, body := ToHTTPResponse(errorspkg.New("boom"))
	if statusCode != 500 {
		t.Fatalf("expected internal server error, got: %d", statusCode)
	}
	if body["msg"] != "internal server error" {
		t.Fatalf("unexpected body: %#v", body)
	}
	if got := buf.String(); !strings.Contains(got, "ToHTTPResponse: non-BizError discarded in response: boom") {
		t.Fatalf("expected fallback log output, got: %q", got)
	}
}
