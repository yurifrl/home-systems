# syntax=docker/dockerfile:1
ARG PYTHON_VERSION=3.13
FROM python:${PYTHON_VERSION}-slim

WORKDIR /app
RUN pip install --no-cache-dir \
      google-cloud-pubsub==2.* \
      prometheus_client==0.*
COPY main.py weekly.py ./

USER 1000
EXPOSE 9109
ENTRYPOINT ["python", "/app/main.py"]
