# C4 Container — Marketplace

## Основной артефакт

`marketplace-container.puml` — исходник C4 Container диаграммы в PlantUML с подключением библиотеки C4-PlantUML.

`c4-container-color-preview.png` — подробное цветное превью диаграммы для удобного просмотра.

## Рендеринг

Файл `.puml` можно открыть в PlantUML-совместимом редакторе, например через расширение PlantUML для VS Code или IntelliJ IDEA.

Исходник использует C4-PlantUML через `!includeurl`, поэтому при рендере нужен доступ к репозиторию библиотеки C4-PlantUML.

## Что отражено на диаграмме

- покупатель и продавец;
- Marketplace как System Boundary;
- API Gateway;
- User, Catalog, Feed, Order, Payment и Notification Service;
- отдельная БД каждого сервиса;
- Apache Kafka Event Broker;
- внешний платёжный провайдер;
- внешние каналы Email / SMS / Push;
- синхронные HTTP/REST-взаимодействия;
- асинхронные Kafka-события;
- Database per Service;
- source of truth для доменных данных;
- основной checkout flow;
- архитектурные решения и ограничения.
