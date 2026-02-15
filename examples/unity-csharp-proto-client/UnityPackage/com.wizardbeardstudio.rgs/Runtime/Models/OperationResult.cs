namespace WizardBeardStudio.Rgs.Models
{
    public class OperationResult
    {
        public bool Success { get; set; }
        public string ResultCode { get; set; } = string.Empty;
        public string DenialReason { get; set; } = string.Empty;
        public string RequestId { get; set; } = string.Empty;
        public string ServerTime { get; set; } = string.Empty;
    }
}
