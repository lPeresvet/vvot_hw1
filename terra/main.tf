terraform {
  required_providers {
    yandex = {
      source = "yandex-cloud/yandex"
    }
  }
  required_version = ">= 0.13"
}

provider "yandex" {
  cloud_id = "b1g71e95h51okii30p25"
  folder_id = "b1g163vdicpkeevao9ga"
  service_account_key_file = "./key.json"
}


resource "yandex_storage_bucket" "bucket" {
  bucket = "sluchaev_vvot_ocr_bot_setup"
}

resource "yandex_storage_bucket" "mount_bucket" {
  bucket = "sluchaev_vvot_ocr_bot_mount"
}

resource "yandex_storage_object" "yagpt_setup" {
  bucket = yandex_storage_bucket.bucket.id
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
  name        = "func_bot_terraformed"
  user_hash   = archive_file.zip.output_sha256
  runtime     = "golang121"
  entrypoint  = "index.handler"
  memory      = 128
  execution_timeout  = 10
  environment = {
    "TG_API_KEY" = var.TG_API_KEY,
    "OCR_API_KEY" = var.OCR_API_KEY,
    "YAGPT_API_KEY" = var.YAGPT_API_KEY,
    "IMAGES_BUCKET" = yandex_storage_bucket.bucket.bucket
  }

  mounts {
    name = "images"
    object_storage {
      bucket = yandex_storage_bucket.mount_bucket.bucket
    }
  }

  content {
    zip_filename = archive_file.zip.output_path
  }
}

variable "TG_API_KEY" {
  type = string
  description = "Ключ для тг бота"
}

variable "OCR_API_KEY" {
  type = string
  description = "Ключ для сервиса OCR"
}

variable "YAGPT_API_KEY" {
  type = string
  description = "Ключ для сервиса YAGPT"
}

output "func_url" {
  value = "https://functions.yandexcloud.net/${yandex_function.func.id}"
}

resource "archive_file" "zip" {
  type = "zip"
  output_path = "func.zip"
  source_dir = "./src"
}

resource "null_resource" "curl" {
  provisioner "local-exec" {
    command = "curl `https://api.telegram.org/bot${var.TG_API_KEY}/setWebhook?url=https://functions.yandexcloud.net/${yandex_function.func.id}"
  }
}