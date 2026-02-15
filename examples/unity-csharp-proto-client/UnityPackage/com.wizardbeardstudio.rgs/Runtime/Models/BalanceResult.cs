namespace WizardBeardStudio.Rgs.Models
{
    public sealed class BalanceResult : OperationResult
    {
        public long AvailableMinor { get; set; }
        public long PendingMinor { get; set; }
        public string Currency { get; set; } = string.Empty;
    }
}
