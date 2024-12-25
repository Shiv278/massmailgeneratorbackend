# Use the official Go image with Alpine
FROM golang:1.23-alpine

# Install glibc
RUN apk update && apk add --no-cache libc6-compat

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy the go.mod and go.sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod tidy

# Copy the rest of the application code
COPY . .

# Build the Go app
RUN go build -o app .

# Expose port 8080 to the outside world
EXPOSE 8080

# Run the executable
CMD ["./app"]