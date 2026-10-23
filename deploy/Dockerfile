# syntax=docker/dockerfile:1
# One local development image for the three entrypoints (S01-T02):
# platform-core and market-ingestion (Go) and research-worker (Python).
# Base images are pinned by digest. Nothing secret is in the build context
# (.dockerignore). This is not a production image.

FROM golang:1.26.10-trixie@sha256:909771fef53e3dcb0950255f0c9a8b4728c3b841e61509c7b29b098de7ee02d4 AS go-build
WORKDIR /src
ENV GOTOOLCHAIN=local CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY cmd ./cmd
COPY internal ./internal
COPY db ./db
COPY research/embed.go research/.python-version ./research/
COPY research/src/tradestocker_research/financials.py ./research/src/tradestocker_research/
RUN go build -trimpath -buildvcs=false -o /out/ ./cmd/platform-core ./cmd/market-ingestion

FROM python:3.14.8-slim@sha256:fbecd461fc72d21b2143c6eaba25143738079154a6d0ea3da3aec724f50eed4a AS wheel-build
COPY tools/uv-requirements.txt /tmp/uv-requirements.txt
RUN python -m pip install --no-cache-dir --disable-pip-version-check --require-hashes --only-binary :all: -r /tmp/uv-requirements.txt
COPY research /src/research
RUN uv build --offline --project /src/research --python /usr/local/bin/python3.14 --wheel --out-dir /out

FROM python:3.14.8-slim@sha256:fbecd461fc72d21b2143c6eaba25143738079154a6d0ea3da3aec724f50eed4a
RUN useradd --system --uid 10001 --user-group --home-dir /nonexistent --shell /usr/sbin/nologin tradestocker \
 && mkdir /results && chown 10001:10001 /results
RUN --mount=type=bind,from=wheel-build,source=/out,target=/wheels \
    python -m pip install --no-cache-dir --disable-pip-version-check --no-index --no-deps /wheels/tradestocker_research-0.1.0-py3-none-any.whl
COPY --from=go-build /out/platform-core /out/market-ingestion /usr/local/bin/
ENV TS_PYTHON=/usr/local/bin/python3.14 TS_MODE=PAPER
USER 10001:10001
