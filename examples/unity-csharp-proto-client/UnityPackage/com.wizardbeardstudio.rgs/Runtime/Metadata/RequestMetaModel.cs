namespace WizardBeardStudio.Rgs.Metadata
{
    public sealed class RequestMetaModel
    {
        public string RequestId { get; set; } = string.Empty;
        public string IdempotencyKey { get; set; } = string.Empty;
        public ActorContext Actor { get; set; } = new ActorContext(string.Empty, RgsActorType.Unspecified);
        public string DeviceId { get; set; } = string.Empty;
        public string UserAgent { get; set; } = string.Empty;
        public string Geo { get; set; } = string.Empty;
    }
}
