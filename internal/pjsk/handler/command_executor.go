package handler

import (
	"context"
	"fmt"
	"time"

	"haruki-cloud/internal/onebot11"
	renderapp "haruki-cloud/internal/pjsk/render/app"
)

func wrapRequestExecutor(fn func(*RequestContext) (onebot11.Message, error)) commandExecutor {
	return func(runtime *ExecutionRuntime) (onebot11.Message, error) {
		if runtime == nil || runtime.Request == nil {
			return nil, fmt.Errorf("nil execution runtime")
		}
		return fn(runtime.Request)
	}
}

func bindRequestExecutor(handler HarukiSekaiCommandHandler, fn func(*RequestContext) (onebot11.Message, error)) HarukiSekaiCommandHandler {
	handler.executor = wrapRequestExecutor(fn)
	return handler
}

func ExecuteCommandRequest(ctx context.Context, resolved *CommandRequest, app *renderapp.App) (message onebot11.Message, err error) {
	tPrepare := time.Now()
	runtime, shortCircuit, err := PrepareExecutionRuntime(ctx, resolved, app)
	recordCommandStage(ctx, "runtime_prepare", time.Since(tPrepare))
	if err != nil {
		return nil, err
	}
	if shortCircuit != nil {
		return shortCircuit, nil
	}

	if resolved.executor == nil {
		return nil, fmt.Errorf("command executor is not bound: module=%v mode=%s", resolved.Module, resolved.Mode)
	}

	tExecute := time.Now()
	message, err = resolved.executor(runtime)
	recordCommandStage(runtime.Context, "executor", time.Since(tExecute))
	if err != nil {
		return nil, WrapDomainError(err)
	}
	return message, nil
}
