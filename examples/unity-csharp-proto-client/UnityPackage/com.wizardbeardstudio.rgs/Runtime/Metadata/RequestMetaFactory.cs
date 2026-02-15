using System;

namespace WizardBeardStudio.Rgs.Metadata
{
    public sealed class RequestMetaFactory
    {
        private readonly string _deviceId;
        private readonly string _userAgent;
        private readonly string _geo;

        public RequestMetaFactory(string deviceId, string userAgent, string geo)
        {
            _deviceId = deviceId;
            _userAgent = userAgent;
            _geo = geo;
        }

        public RequestMetaModel Create(ActorContext actor, string idempotencyKey)
        {
            return new RequestMetaModel
            {
                RequestId = Guid.NewGuid().ToString(),
                IdempotencyKey = idempotencyKey,
                Actor = actor,
                DeviceId = _deviceId,
                UserAgent = _userAgent,
                Geo = _geo,
            };
        }
    }
}
