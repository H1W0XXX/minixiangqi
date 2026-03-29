# 迷你象棋

这是一个本地运行的迷你象棋项目，规则接近中国象棋，但做了小型化改造，适合在电脑上直接体验和测试。

## 规则简介

- 棋盘大小为 `7 x 7`
- 基本走法整体参考中国象棋
- 兵在开局时就可以向 `前 / 左 / 右` 行走，不需要过河后才获得横向移动能力

## 模型说明

项目支持两种神经网络后端：

- `ONNX`：使用 `KataGo` 风格导出的 ONNX 模型，默认文件为 `minixiangqi.onnx`
- `NNUE`：使用 [Fairy-Stockfish](https://fairy-stockfish.github.io/) 体系相关的 NNUE 模型，默认文件为 `minixiangqi.nnue`

## 运行方式

目前只考虑在电脑上运行，不面向手机端部署。

如果已经准备好可执行文件，可以直接运行：

```powershell
.\minixiangqi.exe
```

如果需要用 Go 启动：

```powershell
go run .\cmd\minixiangqi-local
```

程序启动后会在本机打开浏览器，默认地址为：

```text
http://127.0.0.1:2888
```

## 默认文件

- `minixiangqi.onnx`：ONNX 模型
- `minixiangqi.nnue`：NNUE 模型
- `onnxruntime.dll`：ONNX Runtime 动态库

## 说明

这是一个本地电脑端的小型象棋实验项目，重点是规则验证、对弈体验和不同模型后端的接入测试。
