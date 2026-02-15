using System;
using UnityEngine;

namespace WizardBeardStudio.Rgs
{
    public enum RgsTransportMode
    {
        GrpcWeb,
        RestGateway,
    }

    [Serializable]
    public sealed class RgsClientConfig
    {
        public string baseUrl = "https://localhost:8080";
        public RgsTransportMode transportMode = RgsTransportMode.GrpcWeb;
        public string playerId = "player-1";
        public string deviceId = "unity-slot-client-01";
        public string userAgent = "unity-slot-client";
        public string geo = string.Empty;
        public int requestTimeoutSeconds = 30;
        public bool autoGenerateIdempotencyKey = true;

        public bool Validate(out string error)
        {
            if (string.IsNullOrWhiteSpace(baseUrl))
            {
                error = "Base URL is required.";
                return false;
            }
            if (string.IsNullOrWhiteSpace(playerId))
            {
                error = "Player ID is required.";
                return false;
            }
            if (requestTimeoutSeconds <= 0)
            {
                error = "Request timeout must be greater than 0.";
                return false;
            }

            error = string.Empty;
            return true;
        }
    }
}
