FROM alpine:latest

# Install required packages
RUN apk add --no-cache \
    bash \
    curl \
    bc

# Create version file
RUN echo "$(date +%Y%m%d_%H%M%S)" > /version.txt

# Create app directory
WORKDIR /app

# Copy the shell script
COPY uptime-shell.sh .

# Make the script executable
RUN chmod +x uptime-shell.sh

# Run the script
CMD ["./uptime-shell.sh"]
