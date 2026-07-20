# ---- build stage ----
FROM golang:1.25-alpine AS build

WORKDIR /src

# Copy the full source, then download and verify dependencies against the
# committed go.mod/go.sum before building.
COPY . .
RUN go mod download && go mod verify

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/dropcrate .

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/dropcrate /dropcrate

EXPOSE 8080 9090
USER nonroot:nonroot
ENTRYPOINT ["/dropcrate"]
CMD ["serve"]
