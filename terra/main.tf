terraform {
  required_providers {
    yandex = {
      source = "yandex-cloud/yandex"
    }
  }
  required_version = ">= 0.13"
}

resource "yandex_iam_service_account" "func-bot-account" {
  name        = "func-bot-account"
  description = "Аккаунт для функции"
  folder_id   = var.folder_id
}

resource "yandex_resourcemanager_folder_iam_binding" "mount-iam" {
  folder_id = var.folder_id
  role               = "storage.admin"

  members = [
    "serviceAccount:${yandex_iam_service_account.func-bot-account.id}",
  ]
}

resource "yandex_resourcemanager_folder_iam_binding" "ocr-iam" {
  folder_id = var.folder_id
  role               = "ai.vision.user"

  members = [
    "serviceAccount:${yandex_iam_service_account.func-bot-account.id}",
  ]
}

resource "yandex_resourcemanager_folder_iam_binding" "yagpt-iam" {
  folder_id = var.folder_id
  role               = "ai.languageModels.user"

  members = [
    "serviceAccount:${yandex_iam_service_account.func-bot-account.id}",
  ]
}

resource "yandex_resourcemanager_folder_iam_binding" "func-admin-iam" {
  folder_id = var.folder_id
  role               = "serverless.functions.admin"

  members = [
    "serviceAccount:${yandex_iam_service_account.func-bot-account.id}",
  ]
}

provider "yandex" {
  cloud_id = var.cloud_id
  folder_id = var.folder_id
  service_account_key_file = "/home/www/.yc-keys/key.json"
}

resource "yandex_storage_bucket" "mount-bucket" {
  bucket = "sluchaev-vvot-ocr-bot-mount"
  folder_id = var.folder_id
}

resource "yandex_storage_object" "yagpt_setup" {
  bucket = yandex_storage_bucket.mount-bucket.id
  key    = "setup.txt"
  source = "./setup.txt"
}

resource "yandex_function_iam_binding" "function-iam" {
  function_id = yandex_function.func.id
  role        = "serverless.functions.invoker"

  members = [
    "system:allUsers",
  ]
}

resource "yandex_function" "func" {
  name        = "func-bot-terraformed"
  user_hash   = archive_file.zip.output_sha256
  runtime     = "golang121"
  entrypoint  = "index.Handler"
  memory      = 128
  execution_timeout  = 10
  environment = {
    "TG_API_KEY" = var.TG_API_KEY,
    "IMAGES_BUCKET" = yandex_storage_bucket.mount-bucket.bucket
  }
  service_account_id = yandex_iam_service_account.func-bot-account.id

  storage_mounts {
    mount_point_name = "images"
    bucket = yandex_storage_bucket.mount-bucket.bucket
    prefix           = ""
  }

  content {
    zip_filename = archive_file.zip.output_path
  }

  provisioner "local-exec" {
    when    = destroy
    command = "curl --insecure -X POST https://api.telegram.org/bot${var.TG_API_KEY}/deleteWebhook"
  }
}

variable "TG_API_KEY" {
  type = string
  description = "Ключ для тг бота"
}

variable "cloud_id" {
  type = string
  description = "Идентификатор облака"
}

variable "folder_id" {
  type = string
  description = "Идентификатор каталога"
}

output "func_url" {
  value = "https://functions.yandexcloud.net/${yandex_function.func.id}"
}

resource "archive_file" "zip" {
  type = "zip"
  output_path = "src.zip"
  source_dir = "internal"
}

resource "null_resource" "curl" {
  provisioner "local-exec" {
    command = "curl --insecure -X POST https://api.telegram.org/bot${var.TG_API_KEY}/setWebhook?url=https://functions.yandexcloud.net/${yandex_function.func.id}"
  }
}
