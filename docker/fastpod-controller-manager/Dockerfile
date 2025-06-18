FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.24.1 as build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/fastpod-controller-manager/main.go


# FROM busybox:1.36.1-glibc
FROM gcr.io/distroless/static:nonroot
WORKDIR /

COPY --from=build /workspace/manager .

# CMD ["fastpodcontrollermanager", "-alsologtostderr", "-v=4"]
CMD [ "/manager", "-alsologtostderr"]









