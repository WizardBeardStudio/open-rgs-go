namespace WizardBeardStudio.Rgs.Auth
{
    public sealed class InMemoryTokenStore : ITokenStore
    {
        public string? AccessToken { get; private set; }
        public string? RefreshToken { get; private set; }

        public void Save(string accessToken, string refreshToken)
        {
            AccessToken = accessToken;
            RefreshToken = refreshToken;
        }

        public void Clear()
        {
            AccessToken = null;
            RefreshToken = null;
        }
    }
}
