FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o main .

FROM alpine:3.18

# Install ca-certificates and run update-ca-certificates
# EDUCATIONAL NOTE: Compiled languages like Go verify certificates strictly using
# the OS root trust store. Installing ca-certificates and mounting mockdock-ca.crt
# ensures Go's internal x509 validation module trusts MockDock SSL interceptors.
RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /app/main .

# We run update-ca-certificates during bootstrap to load mounted CAs
ENTRYPOINT ["sh", "-c", "update-ca-certificates && ./main"]
