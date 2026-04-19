# detector/detector.py

import torch
from PIL import Image
from typing import Optional, Tuple, Dict, Any, Union
from src.model import EfficientNetCustom
from src.utils import get_preprocessing, compute_frequency_channel


class DeepFakeDetector:
    def __init__(self, weights_path: str, device: Optional[torch.device] = None) -> None:
        """
        Инициализация детектора.
        :param weights_path: Путь к файлу с весами (.pth)
        :param device: Устройство для вычислений (cpu или cuda)
        """
        self.device: torch.device = device if device else torch.device('cuda' if torch.cuda.is_available() else 'cpu')

        self.model: EfficientNetCustom = EfficientNetCustom(num_classes=1)

        checkpoint: Union[Dict[str, Any], torch.Tensor] = torch.load(weights_path, map_location=self.device)

        if isinstance(checkpoint, dict):
            if 'model_state_dict' in checkpoint:
                state_dict = checkpoint['model_state_dict']
            elif 'state_dict' in checkpoint:
                state_dict = checkpoint['state_dict']
            else:
                state_dict = checkpoint
        else:
            state_dict = checkpoint

        self.model.load_state_dict(state_dict)
        self.model.to(self.device)
        self.model.eval()

        self.preprocess = get_preprocessing()
        self.threshold: float = 0.70

    def detect(self, image_path: str) -> Tuple[str, float]:
        """
        Метод для детекта.
        :param image_path: Путь к изображению
        :return: Кортеж (Метка 'FAKE'/'REAL', Уверенность 0.0-1.0)
        """
        res = self.predict(image_path)
        return res["label"], res["confidence"]

    def predict(self, image_path: str) -> Dict[str, Any]:
        """
        Полный анализ изображения с возвратом словаря.
        """
        image: Image.Image = Image.open(image_path).convert('RGB')
        img_tensor: torch.Tensor = self.preprocess(image).unsqueeze(0).to(self.device)

        freq_tensor: torch.Tensor = compute_frequency_channel(img_tensor)

        with torch.no_grad():
            output: torch.Tensor = self.model(img_tensor, freq_channel=freq_tensor)
            prob: float = torch.sigmoid(output).item()

        is_fake: bool = prob >= self.threshold
        label: str = "FAKE" if is_fake else "REAL"

        confidence: float = prob if is_fake else (1.0 - prob)

        return {
            "label": label,
            "confidence": confidence,
            "raw_probability": prob,
        }
