namespace WizardBeardStudio.Rgs.Diagnostics
{
    public interface IRgsLogger
    {
        void Info(string message);
        void Warn(string message);
        void Error(string message);
    }
}
