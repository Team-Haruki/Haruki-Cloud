from fastapi import FastAPI
from api.bot.router import bot_router
from api.pjsk.router import pjsk_router
from api.chunithm.router import chunithm_router

app = FastAPI(docs_url=None, redoc_url=None, openapi_url=None)
app.include_router(bot_router)
app.include_router(pjsk_router)
app.include_router(chunithm_router)
