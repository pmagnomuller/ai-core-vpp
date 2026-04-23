FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1

WORKDIR /app

COPY optimizer/requirements.txt ./requirements.txt
RUN pip install --no-cache-dir -r requirements.txt

COPY optimizer/server.py ./server.py
COPY optimizer/gen ./gen

EXPOSE 50051 8081

CMD ["python", "-u", "server.py"]
