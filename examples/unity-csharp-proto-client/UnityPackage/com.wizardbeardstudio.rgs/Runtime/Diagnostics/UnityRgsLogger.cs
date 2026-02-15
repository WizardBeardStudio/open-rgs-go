using UnityEngine;

namespace WizardBeardStudio.Rgs.Diagnostics
{
    public sealed class UnityRgsLogger : IRgsLogger
    {
        public void Info(string message) => Debug.Log("[RGS] " + message);
        public void Warn(string message) => Debug.LogWarning("[RGS] " + message);
        public void Error(string message) => Debug.LogError("[RGS] " + message);
    }
}
