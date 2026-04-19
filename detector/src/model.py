import torch
import torch.nn as nn
import math
from typing import Optional, List, Tuple


class Swish(nn.Module):
    def forward(self, x: torch.Tensor) -> torch.Tensor:
        return x * torch.sigmoid(x)


class SEBlock(nn.Module):
    def __init__(self, in_ch: int, se_ratio: float = 0.25):
        super().__init__()
        se_ch = max(1, int(in_ch * se_ratio))
        self.pool = nn.AdaptiveAvgPool2d(1)
        self.fc = nn.Sequential(
            nn.Linear(in_ch, se_ch),
            Swish(),
            nn.Linear(se_ch, in_ch),
            nn.Sigmoid()
        )

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        b, c, _, _ = x.shape
        y = self.pool(x).view(b, c)
        y = self.fc(y).view(b, c, 1, 1)
        return x * y


def drop_connect(x: torch.Tensor, drop_rate: float, training: bool) -> torch.Tensor:
    if not training or drop_rate == 0.0:
        return x
    keep_prob = 1.0 - drop_rate
    mask = torch.rand(x.shape[0], 1, 1, 1, device=x.device) < keep_prob
    return x / keep_prob * mask


class MBConvBlock(nn.Module):
    def __init__(self, in_ch: int, out_ch: int, k: int, stride: int,
                 expand_ratio: int, se_ratio: float, drop_rate: float):
        super().__init__()
        self.use_residual = (in_ch == out_ch and stride == 1)
        hidden_ch = in_ch * expand_ratio
        self.drop_rate = drop_rate
        layers = []
        if expand_ratio != 1:
            layers += [
                nn.Conv2d(in_ch, hidden_ch, 1, bias=False),
                nn.BatchNorm2d(hidden_ch, eps=1e-3, momentum=0.01),
                Swish()
            ]
        layers += [
            nn.Conv2d(hidden_ch, hidden_ch, k, stride=stride, padding=k // 2, groups=hidden_ch, bias=False),
            nn.BatchNorm2d(hidden_ch, eps=1e-3, momentum=0.01),
            Swish()
        ]
        layers.append(SEBlock(hidden_ch, se_ratio))
        layers += [
            nn.Conv2d(hidden_ch, out_ch, 1, bias=False),
            nn.BatchNorm2d(out_ch, eps=1e-3, momentum=0.01)
        ]
        self.block = nn.Sequential(*layers)

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        out = self.block(x)
        if self.use_residual:
            out = x + drop_connect(out, self.drop_rate, self.training)
        return out


class EfficientNetCustom(nn.Module):
    def __init__(self, num_classes: int = 1, dropout: float = 0.3):
        super().__init__()
        width_mult = 1.1
        depth_mult = 1.1
        # (exp, k, out, reps, stride)
        cfg: List[Tuple[int, int, int, int, int]] = [
            (1, 3, 16, 1, 1), (6, 3, 24, 2, 2), (6, 5, 40, 2, 2),
            (6, 3, 80, 3, 2), (6, 5, 112, 3, 1), (6, 5, 192, 4, 2),
            (6, 3, 320, 1, 1),
        ]

        def round_ch(ch: float) -> int:
            ch *= width_mult
            return max(8, int(ch + 4) // 8 * 8)

        def round_rep(r: int) -> int:
            return int(math.ceil(r * depth_mult))

        stem_ch = round_ch(32)
        self.stem = nn.Sequential(
            nn.Conv2d(4, stem_ch, 3, stride=2, padding=1, bias=False),
            nn.BatchNorm2d(stem_ch, eps=1e-3, momentum=0.01),
            Swish()
        )

        blocks = []
        in_ch = stem_ch
        total_blocks = sum(round_rep(r) for _, _, _, r, _ in cfg)
        idx = 0
        for exp, k, out, reps, stride in cfg:
            out_ch = round_ch(out)
            for i in range(round_rep(reps)):
                s = stride if i == 0 else 1
                drop = 0.2 * idx / total_blocks
                blocks.append(MBConvBlock(in_ch, out_ch, k, s, exp, 0.25, drop))
                in_ch = out_ch
                idx += 1
        self.blocks = nn.Sequential(*blocks)

        head_ch = round_ch(1280)
        self.head = nn.Sequential(
            nn.Conv2d(in_ch, head_ch, 1, bias=False),
            nn.BatchNorm2d(head_ch, eps=1e-3, momentum=0.01),
            Swish()
        )
        self.classifier = nn.Sequential(
            nn.AdaptiveAvgPool2d(1),
            nn.Flatten(),
            nn.Dropout(dropout),
            nn.Linear(head_ch, num_classes)
        )

    def forward(self, x: torch.Tensor, freq_channel: Optional[torch.Tensor] = None) -> torch.Tensor:
        if freq_channel is not None:
            x = torch.cat([x, freq_channel], dim=1)

        x = self.stem(x)
        x = self.blocks(x)
        x = self.head(x)
        return self.classifier(x)
