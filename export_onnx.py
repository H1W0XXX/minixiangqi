import argparse
import os
import sys

import torch

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
if hasattr(sys.stderr, "reconfigure"):
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")


def parse_args():
    parser = argparse.ArgumentParser(
        description="Export animalchess3 checkpoint to a dynamic-batch ONNX model."
    )
    parser.add_argument(
        "--trainsgd-dir",
        default=r"\\wsl.localhost\kawaii_linkage\home\KataGomo-AnimalChess2025\scripts\animalchess3\trainsgd",
        help="Path to animalchess3/trainsgd directory.",
    )
    parser.add_argument(
        "--checkpoint",
        default=(
            r"\\wsl.localhost\kawaii_linkage\home\KataGomo-AnimalChess2025\scripts"
            r"\animalchess3\data\models\b10c384n_sgd-s3985408-d4597511\model.ckpt"
        ),
        help="Path to model.ckpt.",
    )
    parser.add_argument(
        "--output",
        default="minixiangqi.onnx",
        help="Output ONNX file path.",
    )
    parser.add_argument(
        "--pos-len",
        type=int,
        default=7,
        help="Board size used by the model.",
    )
    parser.add_argument(
        "--use-swa",
        action="store_true",
        help="Use SWA model if present in checkpoint.",
    )
    parser.add_argument(
        "--fixed-batch",
        action="store_true",
        help="Export fixed batch=1 instead of dynamic batch.",
    )
    parser.add_argument(
        "--opset",
        type=int,
        default=17,
        help="ONNX opset version.",
    )
    return parser.parse_args()


def ensure_import_path(trainsgd_dir: str):
    if not os.path.isdir(trainsgd_dir):
        raise FileNotFoundError(f"trainsgd directory not found: {trainsgd_dir}")
    if trainsgd_dir not in sys.path:
        sys.path.append(trainsgd_dir)


class ExportWrapper(torch.nn.Module):
    def __init__(self, model):
        super().__init__()
        self.model = model

    def forward(self, bin_inputs, global_inputs):
        outputs = self.model(bin_inputs, global_inputs)
        policy = outputs[0][0][:, 0, :]
        value = outputs[0][1]
        return policy, value


def main():
    args = parse_args()
    ensure_import_path(args.trainsgd_dir)

    from load_model import load_model  # pylint: disable=import-error

    print(f"Loading checkpoint: {args.checkpoint}")
    model, swa_model, _ = load_model(
        args.checkpoint,
        use_swa=args.use_swa,
        device="cpu",
        pos_len=args.pos_len,
        verbose=False,
    )
    target_model = swa_model.module if (args.use_swa and swa_model is not None) else model
    target_model.eval()

    total_params = sum(p.numel() for p in target_model.parameters())
    print(f"Model loaded. Total parameters: {total_params}")
    print(f"Model spatial input shape: {tuple(target_model.bin_input_shape)}")
    print(f"Model global input shape: {tuple(target_model.global_input_shape)}")

    wrapper = ExportWrapper(target_model)
    wrapper.eval()

    bin_channels, bin_h, bin_w = target_model.bin_input_shape
    if bin_h != args.pos_len or bin_w != args.pos_len:
        raise ValueError(
            "Checkpoint board size mismatch: "
            f"model expects {bin_h}x{bin_w}, but --pos-len={args.pos_len}."
        )
    dummy_bin = torch.randn(1, bin_channels, bin_h, bin_w)
    dummy_global = torch.randn(1, target_model.global_input_shape[0])

    with torch.no_grad():
        sample_policy, sample_value = wrapper(dummy_bin, dummy_global)
    print(f"Sample policy shape: {tuple(sample_policy.shape)}")
    print(f"Sample value shape: {tuple(sample_value.shape)}")

    dynamic_axes = None
    if not args.fixed_batch:
        dynamic_axes = {
            "bin_inputs": {0: "batch_size"},
            "global_inputs": {0: "batch_size"},
            "policy": {0: "batch_size"},
            "value": {0: "batch_size"},
        }
        print("Export mode: dynamic batch.")
    else:
        print("Export mode: fixed batch=1.")

    print(f"Exporting ONNX -> {args.output}")
    with torch.no_grad():
        torch.onnx.export(
            wrapper,
            (dummy_bin, dummy_global),
            args.output,
            export_params=True,
            opset_version=args.opset,
            do_constant_folding=True,
            input_names=["bin_inputs", "global_inputs"],
            output_names=["policy", "value"],
            dynamic_axes=dynamic_axes,
            dynamo=False,
        )

    size = os.path.getsize(args.output)
    print(f"SUCCESS: {args.output} ({size / 1024 / 1024:.2f} MB)")


if __name__ == "__main__":
    main()
