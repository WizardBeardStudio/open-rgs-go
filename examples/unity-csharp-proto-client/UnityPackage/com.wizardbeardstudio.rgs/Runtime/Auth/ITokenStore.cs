namespace WizardBeardStudio.Rgs.Auth
{
    public interface ITokenStore
    {
        string? AccessToken { get; }
        string? RefreshToken { get; }
        void Save(string accessToken, string refreshToken);
        void Clear();
    }
}
