from detector import DeepFakeDetector

import json
import os
import time
from kafka import KafkaConsumer, KafkaProducer

import datetime
import socket


HOSTNAME = socket.gethostname()

def log_event(req_id, message):
    timestamp = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    print(f"[{timestamp}] [{HOSTNAME}] [{req_id}] DETECTOR: {message}", flush=True)

def main() -> None:
    weights_path: str = "weights/model_weights.pth"

    try:
        detector = DeepFakeDetector(weights_path=weights_path)
        print("Модель успешно загружена и готова к работе.")
    except Exception as e:
        print(f"Ошибка инициализации: {e}")
        return
    
    INPUT_TOPIC = 'images_ready'
    OUTPUT_TOPIC = 'results'
    KAFKA_SERVER = 'kafka:9092'
    SHARED_DIR = './shared_data'

    consumer = KafkaConsumer(INPUT_TOPIC, bootstrap_servers=KAFKA_SERVER,
                            group_id='detectors-group',
                            value_deserializer=lambda m: json.loads(m.decode('utf-8')))
    producer = KafkaProducer(bootstrap_servers=KAFKA_SERVER,
                            value_serializer=lambda m: json.dumps(m).encode('utf-8'))

    print("Detector instance started...")

    for message in consumer:
        data = message.value
        request_id = data['request_id']
        file_name = data['file_name']

        log_event(request_id, f"Starting analysis for {data['file_name']}")
        
        full_path = os.path.join(SHARED_DIR, file_name)
        
        if os.path.exists(full_path):
            log_event(request_id, f"Loading image into memory: {file_name}")

            start_time = time.time()
            label, confidence = detector.detect(full_path)
            duration = round(time.time() - start_time, 2)

            log_event(request_id, f"Analysis complete in {duration}s. Result: {label} ({confidence})")
            
            # Формируем ответ для Gateway
            result = {
                "request_id": request_id,
                "is_deepfake": label == "FAKE",
                "confidence": confidence
            }
            producer.send(OUTPUT_TOPIC, result)
            log_event(request_id, f"Result sent to {OUTPUT_TOPIC}: {label} ({confidence})")
            
            # Удаляем файл после анализа
            os.remove(full_path)
            # print(f"Processed {request_id}: {label} ({confidence})")
            
        else:
            log_event(request_id, f"ERROR: File {full_path} missing!")

if __name__ == "__main__":
    main()