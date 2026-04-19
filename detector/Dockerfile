FROM python:3.9-slim

# RUN apt-get update && apt-get install -y \
#     libgl1-mesa-glx \
#     libglib2.0-0 \
#     && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .

RUN pip install --no-cache-dir --progress-bar on -r requirements.txt

COPY . .

# Создаем папку для обмена данными
RUN mkdir -p ./shared_data

# Запускаем скрипт
CMD ["python", "-u", "main.py"]