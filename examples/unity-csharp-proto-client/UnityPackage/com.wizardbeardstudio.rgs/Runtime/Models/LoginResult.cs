namespace WizardBeardStudio.Rgs.Models
{
    public sealed class LoginResult : OperationResult
    {
        public string? AccessToken { get; set; }
        public string? RefreshToken { get; set; }
        public string? ActorId { get; set; }
    }
}
