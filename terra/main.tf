terraform {
  required_providers {
    yandex = {
      source = "yandex-cloud/yandex"
    }
  }
  required_version = ">= 0.13"
}

# TODO move locals to input variables
locals {
  cloud_id = "b1g71e95h51okii30p25"
  folder_id = "b1g163vdicpkeevao9ga"
}

provider "yandex" {
  cloud_id = local.cloud_id
  folder_id = local.folder_id
  service_account_key_file = "/home/www/.yc-keys/key.json"
}

resource "yandex_storage_bucket" "mount-bucket" {
  bucket = "sluchaev-vvot-ocr-bot-mount"
  folder_id = local.folder_id
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
    "OCR_API_KEY" = var.OCR_API_KEY,
    "YAGPT_API_KEY" = var.YAGPT_API_KEY,
    "IMAGES_BUCKET" = yandex_storage_bucket.mount-bucket.bucket
  }
  service_account_id = "ajeqrhod65lvpvvagmus"

  storage_mounts {
    mount_point_name = "images"
    bucket = yandex_storage_bucket.mount-bucket.bucket
    prefix           = ""
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
  output_path = "src.zip"
  source_dir = "internal"
}

resource "null_resource" "curl" {
  provisioner "local-exec" {
    command = "curl --insecure -X POST https://api.telegram.org/bot${var.TG_API_KEY}/setWebhook?url=https://functions.yandexcloud.net/${yandex_function.func.id}"
  }
}

#resource "yandex_iam_service_account" "sa" {
#  name        = "<service_account_name>"
#  description = "<service_account_description>"
#  folder_id   = "<folder_ID>"
#}

#resource "yandex_resourcemanager_folder_iam_member" "admin-account-iam" {
#  folder_id   = "<folder_ID>"
#  role        = "<role>"
#  member      = "serviceAccount:<service_account_ID>"
#}