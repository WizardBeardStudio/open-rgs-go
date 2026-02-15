using System;
using System.Threading;
using System.Threading.Tasks;
using WizardBeardStudio.Rgs.Models;

namespace WizardBeardStudio.Rgs.Services
{
    public sealed class ReportingClient
    {
        public Task<OperationResult> GenerateReportAsync(string reportType, string interval, CancellationToken cancellationToken)
        {
            throw new NotImplementedException("Reporting client implementation should bridge generated protobuf clients.");
        }
    }
}
