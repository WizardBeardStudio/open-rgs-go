namespace WizardBeardStudio.Rgs.Auth
{
    public interface ITokenStore
    {
        string? AccessToken { get; }
        string? RefreshToken { get; }
        string? ActorId { get; }
        void Save(string accessToken, string refreshToken, string actorId);
        void Clear();
    }
}
