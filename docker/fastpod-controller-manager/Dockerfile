FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.22 as build

ENV GOLANG_VERSION 1.22.0
ENV PATH $PATH:/usr/local/go/bin

WORKDIR /go/src/github.com/KontonGu/FaST-GShare

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN make fastpod-controller-manager


# FROM busybox:1.36.1-glibc
FROM  ubuntu:22.04

ENV NVIDIA_VISIBLE_DEVICES=all
ENV NVIDIA_DRIVER_CAPABILITIES=utility

COPY --from=build /go/src/github.com/KontonGu/FaST-GShare/cmd/fastpod-controller-manager/fastpodcontrollermanager /usr/bin/fastpodcontrollermanager

# CMD ["fastpodcontrollermanager", "-alsologtostderr", "-v=4"]
CMD [ "fastpodcontrollermanager", "-alsologtostderr"]









