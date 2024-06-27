# Сборка приложения
FROM golang:1.22-alpine AS application

ARG GITHUB_REF
ADD . /bundle

WORKDIR /bundle

RUN \
    revision=${GITHUB_REF} && \
    echo "Building container. Revision: ${revision}" && \
    go build -ldflags "-X main.build=${revision}" -o /srv/app ./main.go

# Финальная сборка образа
FROM scratch
COPY --from=application /srv /srv
EXPOSE 8080
WORKDIR /srv
ENTRYPOINT ["/srv/app"]