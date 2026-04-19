import json
import os
from PIL import Image
from kafka import KafkaConsumer, KafkaProducer

import datetime

INPUT_TOPIC = 'images_raw'
OUTPUT_TOPIC = 'images_ready'
KAFKA_SERVER = 'kafka:9092'
SHARED_DIR = './shared_data'

consumer = KafkaConsumer(INPUT_TOPIC, bootstrap_servers=KAFKA_SERVER, 
                         value_deserializer=lambda m: json.loads(m.decode('utf-8')))
producer = KafkaProducer(bootstrap_servers=KAFKA_SERVER, 
                         value_serializer=lambda m: json.dumps(m).encode('utf-8'))

print("Converter started...")

def log_event(req_id, message):
    timestamp = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    print(f"[{timestamp}] [{req_id}] CONVERTER: {message}", flush=True)

for message in consumer:
    data = message.value
    request_id = data['request_id']
    file_name = data['file_name']

    log_event(request_id, f"Message received from {INPUT_TOPIC}")
    
    old_path = os.path.join(SHARED_DIR, file_name)
    new_file_name = f"{request_id}.png"
    new_path = os.path.join(SHARED_DIR, new_file_name)

    log_event(request_id, f"Picked up task for file: {data['file_name']}")

    try:
        # Конвертация в PNG
        log_event(request_id, f"Opening image: {file_name}")
        with Image.open(old_path) as img:
            format_orig = img.format
            log_event(request_id, f"Original format: {format_orig}. Converting...")
            img.save(new_path, 'PNG')
        
        log_event(request_id, f"Saved as: {new_file_name}")
        
        # Удаляем старый файл, если он не был PNG
        if old_path != new_path:
            os.remove(old_path)
        
        log_event(request_id, f"Successfully converted {file_name} to PNG")
            
        # Отправляем задачу нейросетям
        producer.send(OUTPUT_TOPIC, {'request_id': request_id, 'file_name': new_file_name})
        log_event(request_id, f"Sent task to topic '{OUTPUT_TOPIC}'")
    except Exception as e:
        log_event(request_id, f"ERROR converting {file_name}: {e}")