package inputs

import (
	"errors"
	"testing"

	klog "k8s.io/klog/v2"
)

type testKlogInitializer struct {
	legacyCalled bool
	loggerCalled bool
	logger       klog.Logger
	err          error
}

func (t *testKlogInitializer) Init() error {
	t.legacyCalled = true
	return nil
}

func (t *testKlogInitializer) InitWithLogger(logger klog.Logger) error {
	t.loggerCalled = true
	t.logger = logger
	return t.err
}

type testLegacyInitializer struct {
	called bool
	err    error
}

func (t *testLegacyInitializer) Init() error {
	t.called = true
	return t.err
}

func TestMayInit(t *testing.T) {
	t.Run("logger-aware initializer is preferred", func(t *testing.T) {
		wantErr := errors.New("logger init failed")
		target := &testKlogInitializer{err: wantErr}
		logger := klog.Background()

		err := MayInit(target, logger)
		if err != wantErr {
			t.Fatalf("expected exact error %v, got %v", wantErr, err)
		}
		if !target.loggerCalled {
			t.Fatal("expected logger-aware initializer to be called")
		}
		if target.legacyCalled {
			t.Fatal("expected legacy initializer to be skipped")
		}
		var zeroLogger klog.Logger
		if target.logger == zeroLogger {
			t.Fatal("expected logger to be passed to logger-aware initializer")
		}
	})

	t.Run("legacy initializer still works", func(t *testing.T) {
		wantErr := errors.New("legacy init failed")
		target := &testLegacyInitializer{err: wantErr}
		logger := klog.Background()

		err := MayInit(target, logger)
		if err != wantErr {
			t.Fatalf("expected exact error %v, got %v", wantErr, err)
		}
		if !target.called {
			t.Fatal("expected legacy initializer to be called")
		}
	})

	t.Run("non-initializer returns nil", func(t *testing.T) {
		if err := MayInit(struct{}{}, klog.Background()); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("errors propagate unchanged", func(t *testing.T) {
		wantErr := errors.New("unchanged error")
		target := &testKlogInitializer{err: wantErr}
		logger := klog.Background()

		err := MayInit(target, logger)
		if err != wantErr {
			t.Fatalf("expected exact error %v, got %v", wantErr, err)
		}
	})
}
