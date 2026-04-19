FROM python:3.9-slim
WORKDIR /app

RUN pip install --no-cache-dir kafka-python-ng Pillow
COPY main.py .

RUN mkdir ./shared_data
CMD ["python", "main.py"]