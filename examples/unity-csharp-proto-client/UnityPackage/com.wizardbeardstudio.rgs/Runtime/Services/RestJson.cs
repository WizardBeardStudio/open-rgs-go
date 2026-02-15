using System;
using System.Text.Json;
using WizardBeardStudio.Rgs.Models;

namespace WizardBeardStudio.Rgs.Services
{
    internal static class RestJson
    {
        internal sealed class ParsedMeta
        {
            public bool Success { get; set; }
            public string ResultCode { get; set; } = string.Empty;
            public string DenialReason { get; set; } = string.Empty;
            public string RequestId { get; set; } = string.Empty;
            public string ServerTime { get; set; } = string.Empty;
        }

        public static ParsedMeta ParseMeta(JsonElement root)
        {
            if (!root.TryGetProperty("meta", out var meta))
            {
                return new ParsedMeta
                {
                    Success = false,
                    ResultCode = "MISSING_META",
                    DenialReason = "missing response metadata",
                };
            }

            var codeString = GetString(meta, "resultCode");
            var success = string.Equals(codeString, "RESULT_CODE_OK", StringComparison.OrdinalIgnoreCase)
                || string.Equals(codeString, "OK", StringComparison.OrdinalIgnoreCase)
                || string.Equals(codeString, "1", StringComparison.OrdinalIgnoreCase);

            if (meta.TryGetProperty("resultCode", out var resultCodeRaw) && resultCodeRaw.ValueKind == JsonValueKind.Number)
            {
                success = resultCodeRaw.GetInt32() == ProtoResultCode.Ok;
                codeString = resultCodeRaw.GetInt32().ToString();
            }

            return new ParsedMeta
            {
                Success = success,
                ResultCode = codeString,
                DenialReason = GetString(meta, "denialReason"),
                RequestId = GetString(meta, "requestId"),
                ServerTime = GetString(meta, "serverTime"),
            };
        }

        public static string GetString(JsonElement element, string property)
        {
            if (!element.TryGetProperty(property, out var value))
            {
                return string.Empty;
            }
            return value.ValueKind switch
            {
                JsonValueKind.String => value.GetString() ?? string.Empty,
                JsonValueKind.Number => value.GetRawText(),
                JsonValueKind.True => "true",
                JsonValueKind.False => "false",
                _ => string.Empty,
            };
        }

        public static long GetInt64(JsonElement element, string property)
        {
            if (!element.TryGetProperty(property, out var value))
            {
                return 0;
            }

            if (value.ValueKind == JsonValueKind.Number)
            {
                return value.GetInt64();
            }
            if (value.ValueKind == JsonValueKind.String && long.TryParse(value.GetString(), out var parsed))
            {
                return parsed;
            }
            return 0;
        }

        public static OperationResult ToOperationResult(ParsedMeta meta)
        {
            return new OperationResult
            {
                Success = meta.Success,
                ResultCode = meta.ResultCode,
                DenialReason = meta.DenialReason,
                RequestId = meta.RequestId,
                ServerTime = meta.ServerTime,
            };
        }
    }
}
