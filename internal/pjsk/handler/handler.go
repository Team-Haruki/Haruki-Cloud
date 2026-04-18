package handler

import corehandler "haruki-cloud/internal/handler"

const DefaultPriority = corehandler.DefaultPriority

type CommandHandler interface {
	corehandler.CommandHandler
	Handle(Context) (*CommandRequest, error)
}

type CommandHandlerBase = corehandler.CommandHandlerBase
