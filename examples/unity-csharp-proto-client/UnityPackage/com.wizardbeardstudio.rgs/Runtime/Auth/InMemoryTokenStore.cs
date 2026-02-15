namespace WizardBeardStudio.Rgs.Auth
{
    public sealed class InMemoryTokenStore : ITokenStore
    {
        public string? AccessToken { get; private set; }
        public string? RefreshToken { get; private set; }
        public string? ActorId { get; private set; }

        public void Save(string accessToken, string refreshToken, string actorId)
        {
            AccessToken = accessToken;
            RefreshToken = refreshToken;
            ActorId = actorId;
        }

        public void Clear()
        {
            AccessToken = null;
            RefreshToken = null;
            ActorId = null;
        }
    }
}
