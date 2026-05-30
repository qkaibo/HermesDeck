FROM python:3.11-slim
RUN apt-get update && apt-get install -y --no-install-recommends git curl && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY src/python/requirements.txt /app/
RUN pip install --no-cache-dir -r /app/requirements.txt
COPY src/python/hermesdeck_sidecar/ /app/hermesdeck_sidecar/
RUN mkdir -p /data
VOLUME ["/data"]
EXPOSE 50052
ENTRYPOINT ["python", "-m", "hermesdeck_sidecar"]
