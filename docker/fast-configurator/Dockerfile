ARG GOLANG_VERSION=1.24.1
FROM  nvidia/cuda:12.2.0-base-ubuntu22.04 AS build
ARG GOLANG_VERSION

# RUN apt-get update && \
#     apt-get install -y wget make git gcc \
#     && \
#     rm -rf /var/lib/apt/lists/*
# RUN set -eux; \
#     \
#     arch="$(uname -m)"; \
#     case "${arch##*-}" in \
#         x86_64 | amd64) ARCH='amd64' ;; \
#         ppc64el | ppc64le) ARCH='ppc64le' ;; \
#         aarch64) ARCH='arm64' ;; \
#         *) echo "unsupported architecture" ; exit 1 ;; \
#     esac; \
#     wget -nv -O - https://storage.googleapis.com/golang/go${GOLANG_VERSION}.linux-${ARCH}.tar.gz \
#     | tar -C /usr/local -xz



ENV GOPATH=/go
ENV PATH=$GOPATH/bin:/usr/local/go/bin:$PATH

WORKDIR /build


# COPY go.mod go.sum ./

# RUN go mod download

# COPY . .



COPY ./main fast-configurator
# executable probably there already
# RUN make fast-configurator



FROM nvidia/cuda:12.2.0-base-ubuntu22.04

# Remove CUDA libs(compat etc) in favor of libs installed by the NVIDIA driver
# RUN rm -f cuda-*.deb
# RUN apt-get --purge -y autoremove cuda-*


ENV NVIDIA_DISABLE_REQUIRE="true"
ENV NVIDIA_VISIBLE_DEVICES=all
ENV NVIDIA_DRIVER_CAPABILITIES=compute,utility

COPY --from=build build/fast-configurator /usr/bin/fast-configurator

CMD [ "fast-configurator", "-alsologtostderr", "-ctr_mgr_ip_port=fastpod-controller-manager-svc.kube-system.svc.cluster.local:10086", "-fastfunc_ip_port=fastfunc-svc.kube-system.svc.cluster.local:10088", "-v=4"]
