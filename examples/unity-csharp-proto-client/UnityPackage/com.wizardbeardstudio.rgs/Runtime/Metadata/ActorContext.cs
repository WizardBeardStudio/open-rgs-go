namespace WizardBeardStudio.Rgs.Metadata
{
    public enum RgsActorType
    {
        Unspecified = 0,
        Player = 1,
        Operator = 2,
        Service = 3,
    }

    public sealed class ActorContext
    {
        public string ActorId { get; }
        public RgsActorType ActorType { get; }

        public ActorContext(string actorId, RgsActorType actorType)
        {
            ActorId = actorId;
            ActorType = actorType;
        }
    }
}
