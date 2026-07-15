package handler

import (
	"context"
	"fmt"

	"haruki-cloud/internal/observability/commandtrace"
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
	if resolved == nil {
		return nil, fmt.Errorf("bridge: nil resolved command")
	}
	if resolved.IsHelp {
		finish := commandtrace.MeasurePhase(ctx, "command_execute")
		defer finish()
		return commandHelpMessage(ctx, resolved, app)
	}

	var runtime *ExecutionRuntime
	var shortCircuit onebot11.Message
	func() {
		finishPrepare := commandtrace.MeasurePhase(ctx, "runtime_prepare")
		defer finishPrepare()
		runtime, shortCircuit, err = PrepareExecutionRuntime(ctx, resolved, app)
	}()
	if err != nil {
		return nil, err
	}
	if shortCircuit != nil {
		return shortCircuit, nil
	}

	if resolved.executor == nil {
		return nil, fmt.Errorf("command executor is not bound: module=%v mode=%s", resolved.Module, resolved.Mode)
	}

	func() {
		finishExecute := commandtrace.MeasurePhase(runtime.Context, "command_execute")
		defer finishExecute()
		message, err = resolved.executor(runtime)
	}()
	if err != nil {
		return nil, WrapDomainError(err)
	}
	return message, nil
}
