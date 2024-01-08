FROM multiarch/qemu-user-static:x86_64-aarch64 as qemu
FROM golang:alpine as gobuilder

WORKDIR /workdir

COPY go.mod go.sum ./
RUN go mod tidy

COPY . .
RUN go build -o hs

# Has the cli binary, hs 
FROM nixos/nix

COPY --from=qemu /usr/bin/qemu-aarch64-static /usr/bin
COPY --from=gobuilder /workdir/hs /usr/local/bin/hs

WORKDIR /workdir

ENV PATH="/result/bin:/usr/local/bin:$PATH"

# Enable extra experimental features for Nix
RUN echo 'extra-experimental-features = nix-command' >> /etc/nix/nix.conf
RUN echo 'extra-experimental-features = flakes' >> /etc/nix/nix.conf
RUN echo 'extra-platforms = aarch64-linux' >> /etc/nix/nix.conf

# Configure Nix and install packages
RUN nix-env -f https://github.com/nix-community/nixos-generators/archive/master.tar.gz -i

# Update the Nix channel
RUN nix-channel --update

# This should be in a flake
RUN nix-env -i vim fish

COPY nix/ .

RUN nix \
    --option filter-syscalls false \
    --show-trace \
    build

RUN mv result /result

ENTRYPOINT ["hs" ]

CMD ["help"]