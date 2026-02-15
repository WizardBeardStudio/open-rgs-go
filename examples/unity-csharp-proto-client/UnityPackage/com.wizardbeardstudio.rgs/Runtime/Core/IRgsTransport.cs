using System.Collections.Generic;
using System.Threading;
using System.Threading.Tasks;

namespace WizardBeardStudio.Rgs.Core
{
    public interface IRgsTransport
    {
        Task<string> PostJsonAsync(string path, string jsonBody, IDictionary<string, string>? headers, CancellationToken cancellationToken);
        Task<string> GetJsonAsync(string path, IDictionary<string, string>? headers, CancellationToken cancellationToken);
    }
}
