FROM python:3.12-slim

WORKDIR /app

# Install dependencies first, separately from copying the rest of the
# source, so Docker can cache this layer and skip reinstalling on every
# code change, only when requirements.txt actually changes.
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

CMD ["sh", "-c", "echo 'Specify a script to run, e.g.: docker compose run python python rag.py \"your question\"'"]
