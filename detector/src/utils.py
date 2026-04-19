import torch
import numpy as np
from scipy import fftpack
from torchvision import transforms
from typing import List


def compute_frequency_channel(image_tensor: torch.Tensor) -> torch.Tensor:
    batch_size, channels, height, width = image_tensor.shape
    freq_channels: List[torch.Tensor] = []

    for i in range(batch_size):
        img_gray = image_tensor[i, 1, :, :].cpu().numpy()

        fft = fftpack.fft2(img_gray)
        fft_shifted = fftpack.fftshift(fft)

        magnitude = np.log(np.abs(fft_shifted) + 1)

        mag_min, mag_max = magnitude.min(), magnitude.max()
        magnitude = (magnitude - mag_min) / (mag_max - mag_min + 1e-8)

        magnitude_tensor = torch.from_numpy(magnitude).float().unsqueeze(0)
        freq_channels.append(magnitude_tensor)

    return torch.stack(freq_channels).to(image_tensor.device)


def get_preprocessing() -> transforms.Compose:
    return transforms.Compose([
        transforms.Resize((256, 256)),
        transforms.ToTensor(),
        transforms.Normalize([0.485, 0.456, 0.406], [0.229, 0.224, 0.225])
    ])