---
title: Machine Learning Basics
date: 2026-02-10
tags: [research, machine-learning, AI, python]
status: evergreen
---

# Machine Learning Basics

A reference note covering foundational ML concepts. Useful for the AI features in Granit (see [[Getting Started]]) and future work on [[Ideas/Side Projects]].

## Core Concepts

### Supervised Learning

The model learns from labeled training data. Given input-output pairs `(x, y)`, it learns a function `f(x) -> y`.

**Common algorithms:**
- Linear Regression
- Logistic Regression
- Decision Trees / Random Forests
- Support Vector Machines (SVM)
- Neural Networks

### Unsupervised Learning

The model finds patterns in unlabeled data. No target variable — the algorithm discovers structure on its own.

**Common algorithms:**
- K-Means Clustering
- DBSCAN
- Principal Component Analysis (PCA)
- Autoencoders
- Gaussian Mixture Models

### Reinforcement Learning

An agent learns by interacting with an environment, receiving rewards or penalties for actions. The goal is to maximize cumulative reward.

> Key concepts: state, action, reward, policy, value function, Q-function

## The ML Pipeline

```
Data Collection → Cleaning → Feature Engineering → Model Training → Evaluation → Deployment
      ↑                                                                    |
      └────────────────────── Feedback Loop ───────────────────────────────┘
```

## Code Examples

### Linear Regression in Python

```python
import numpy as np
from sklearn.linear_model import LinearRegression
from sklearn.model_selection import train_test_split
from sklearn.metrics import mean_squared_error, r2_score

# Generate sample data
np.random.seed(42)
X = np.random.randn(200, 3)
y = 3 * X[:, 0] + 1.5 * X[:, 1] - 2 * X[:, 2] + np.random.randn(200) * 0.5

# Split data
X_train, X_test, y_train, y_test = train_test_split(X, y, test_size=0.2)

# Train model
model = LinearRegression()
model.fit(X_train, y_train)

# Evaluate
y_pred = model.predict(X_test)
print(f"R² Score: {r2_score(y_test, y_pred):.4f}")
print(f"RMSE:     {np.sqrt(mean_squared_error(y_test, y_pred)):.4f}")
print(f"Coefficients: {model.coef_}")
```

### Simple Neural Network with PyTorch

```python
import torch
import torch.nn as nn

class SimpleNet(nn.Module):
    def __init__(self, input_dim, hidden_dim, output_dim):
        super().__init__()
        self.layers = nn.Sequential(
            nn.Linear(input_dim, hidden_dim),
            nn.ReLU(),
            nn.Dropout(0.2),
            nn.Linear(hidden_dim, hidden_dim),
            nn.ReLU(),
            nn.Linear(hidden_dim, output_dim),
        )

    def forward(self, x):
        return self.layers(x)

# Usage
model = SimpleNet(input_dim=10, hidden_dim=64, output_dim=3)
optimizer = torch.optim.Adam(model.parameters(), lr=0.001)
criterion = nn.CrossEntropyLoss()
```

## Evaluation Metrics

| Metric | Use Case | Formula |
|--------|----------|---------|
| Accuracy | Balanced classes | (TP + TN) / Total |
| Precision | Minimize false positives | TP / (TP + FP) |
| Recall | Minimize false negatives | TP / (TP + FN) |
| F1 Score | Balance precision/recall | 2 * P * R / (P + R) |
| AUC-ROC | Binary classification | Area under ROC curve |
| RMSE | Regression | sqrt(mean((y - y_hat)^2)) |

## Key Mathematical Concepts

### Gradient Descent

The optimization algorithm that powers most ML training:

```
theta = theta - learning_rate * gradient(loss, theta)
```

**Variants:**
- **Batch GD** — Uses entire dataset per step. Stable but slow.
- **Stochastic GD** — One sample per step. Noisy but fast.
- **Mini-batch GD** — Compromise: batches of 32-512 samples.

### Bias-Variance Tradeoff

- **High bias** (underfitting): Model too simple, misses patterns
- **High variance** (overfitting): Model too complex, memorizes noise
- **Goal:** Find the sweet spot where both are minimized

## Resources

- *Hands-On Machine Learning* by Aurelien Geron
- [[Books/Designing Data-Intensive Applications]] — Covers data pipelines relevant to ML infrastructure
- Stanford CS229 lecture notes
- [[Research/Graph Databases]] — Graph neural networks use graph DB concepts

## Applications in Granit

The local AI fallback in Granit uses simplified versions of these concepts:
- **TF-IDF** for keyword extraction in the auto-tagger
- **Cosine similarity** for link suggestion
- **Stopword filtering** as a basic NLP preprocessing step
