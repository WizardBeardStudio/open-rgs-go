namespace WizardBeardStudio.Rgs.Models
{
    public sealed class SessionResult : OperationResult
    {
        public string SessionId { get; set; } = string.Empty;
        public string SessionState { get; set; } = string.Empty;
    }
}
