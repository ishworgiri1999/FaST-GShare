FROM --platform=linux/amd64 golang:1.22 AS build


WORKDIR /go/src/github.com/KontonGu/FaST-GShare

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN make fast-configurator



FROM --platform=linux/amd64 nvidia/cuda:12.2.0-base-ubuntu22.04



ENV NVIDIA_VISIBLE_DEVICES=all
ENV NVIDIA_DRIVER_CAPABILITIES=utility

COPY --from=build /go/src/github.com/KontonGu/FaST-GShare/cmd/fast-configurator/fast-configurator /usr/bin/fast-configurator

CMD [ "fast-configurator", "-alsologtostderr", "-ctr_mgr_ip_port=fastpod-controller-manager-svc.kube-system.svc.cluster.local:10086", "-v=4"]
